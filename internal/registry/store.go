package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// DefaultPoolStart and DefaultPoolEnd bound the machine-wide port pool
	// (inclusive) that allocation scans, per DESIGN.md's "default
	// 4000-9999, machine-configurable" pool. Loading a machine config
	// override is out of scope for this package; callers may override
	// Store.PoolStart/PoolEnd directly.
	DefaultPoolStart = 4000
	DefaultPoolEnd   = 9999

	// GCRateLimit is the minimum interval between opportunistic GC runs
	// triggered by create/list, per DESIGN.md's "Command behavior" section.
	GCRateLimit = time.Hour

	registryFileName      = "registry.json"
	lockFileName          = "registry.lock"
	registrySchemaVersion = 1
)

// registryFile is the on-disk JSON schema described in DESIGN.md's "Port
// registry" section, plus a last_gc_at field used to rate-limit
// opportunistic GC.
type registryFile struct {
	Version  int        `json:"version"`
	Claims   []Claim    `json:"claims"`
	LastGCAt *time.Time `json:"last_gc_at,omitempty"`
}

// Store is the machine-global port registry: a JSON file at RegistryPath,
// guarded by an exclusive flock on LockPath for every read-modify-write.
type Store struct {
	RegistryPath string
	LockPath     string
	PoolStart    int
	PoolEnd      int
	PortChecker  PortChecker
}

// Open resolves the registry's state directory via StateDir, creates it if
// necessary, and returns a Store ready for use.
func Open() (*Store, error) {
	stateDir, err := StateDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating state directory %s: %w", stateDir, err)
	}

	return NewStore(stateDir), nil
}

// NewStore builds a Store rooted at the given state directory without
// touching the filesystem, so tests can point it at a temp directory.
func NewStore(stateDir string) *Store {
	return &Store{
		RegistryPath: filepath.Join(stateDir, registryFileName),
		LockPath:     filepath.Join(stateDir, lockFileName),
		PoolStart:    DefaultPoolStart,
		PoolEnd:      DefaultPoolEnd,
		PortChecker:  LsofPortChecker{},
	}
}

// ClaimRequest describes the instance a caller wants a port block for.
type ClaimRequest struct {
	Project    string
	Instance   string
	Repository string
	Worktree   string
	BlockSize  int
}

// ClaimResult is the outcome of a successful Claim call: the block reserved
// (or the pre-existing block, if the request was idempotent) plus any
// non-fatal warnings encountered along the way, such as lsof being
// unavailable.
type ClaimResult struct {
	Claim    Claim
	Existing bool
	Warnings []string
}

// Claim reserves a port block for the given instance. Per DESIGN.md's
// allocation algorithm: take the lock, garbage-collect stale claims, scan
// the pool in block_size steps for the first block that overlaps no
// existing claim and has no port currently listening, write the claim,
// release the lock. A request for a project/instance pair that already has
// a claim is idempotent: it returns the existing claim unchanged.
func (store *Store) Claim(request ClaimRequest) (*ClaimResult, error) {
	if request.BlockSize <= 0 {
		return nil, fmt.Errorf("registry: block size must be positive, got %d", request.BlockSize)
	}

	var result *ClaimResult

	err := store.withLock(func() error {
		registry, err := store.load()
		if err != nil {
			return err
		}

		if existing, ok := findClaim(registry.Claims, request.Project, request.Instance); ok {
			result = &ClaimResult{Claim: existing, Existing: true}
			return nil
		}

		registry.Claims, _ = gcStaleClaims(registry.Claims)

		listening, warnings := store.listeningPortsWithWarnings()

		blockStart, ok := firstFreeBlock(registry.Claims, request.BlockSize, store.PoolStart, store.PoolEnd, listening)
		if !ok {
			return ErrPoolExhausted
		}

		claim := Claim{
			Project:    request.Project,
			Instance:   request.Instance,
			Repository: request.Repository,
			Worktree:   request.Worktree,
			BlockStart: blockStart,
			BlockSize:  request.BlockSize,
			CreatedAt:  time.Now().UTC(),
		}

		// Deliberately does not touch registry.LastGCAt: a claim runs a
		// stale-claim sweep as a side effect of allocation (see
		// gcStaleClaims above), but that is not a real GC pass — only
		// Store.GC's own run (the standalone `gc` command, or the
		// opportunistic pass create/list trigger) should reset the
		// opportunistic-gc rate limit. Otherwise every `create` would
		// silently suppress the next hour's opportunistic gc even though
		// no gc actually ran.
		registry.Claims = append(registry.Claims, claim)

		if err := store.save(registry); err != nil {
			return err
		}

		result = &ClaimResult{Claim: claim, Warnings: warnings}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Release removes the claim for the given project/instance, freeing its
// port block for reuse.
func (store *Store) Release(project, instance string) error {
	return store.withLock(func() error {
		registry, err := store.load()
		if err != nil {
			return err
		}

		var kept []Claim
		found := false
		for _, claim := range registry.Claims {
			if claim.Project == project && claim.Instance == instance {
				found = true
				continue
			}
			kept = append(kept, claim)
		}

		if !found {
			return fmt.Errorf("registry: no claim for project %q instance %q", project, instance)
		}

		registry.Claims = kept

		return store.save(registry)
	})
}

// List returns every claim currently in the registry, without running GC.
func (store *Store) List() ([]Claim, error) {
	var claims []Claim

	err := store.withLock(func() error {
		registry, err := store.load()
		if err != nil {
			return err
		}

		claims = registry.Claims

		return nil
	})
	if err != nil {
		return nil, err
	}

	return claims, nil
}

// GC removes claims whose worktree no longer exists and returns them.
func (store *Store) GC() ([]Claim, error) {
	var removed []Claim

	err := store.withLock(func() error {
		registry, err := store.load()
		if err != nil {
			return err
		}

		var kept []Claim
		kept, removed = gcStaleClaims(registry.Claims)
		registry.Claims = kept

		now := time.Now().UTC()
		registry.LastGCAt = &now

		return store.save(registry)
	})
	if err != nil {
		return nil, err
	}

	return removed, nil
}

// LastGCAt returns the registry's recorded last_gc_at timestamp, or nil if
// GC has never run. It does not take the lock: a slightly stale read is
// fine for the opportunistic-gc rate-limit check NeedsGC is used for.
func (store *Store) LastGCAt() (*time.Time, error) {
	registry, err := store.load()
	if err != nil {
		return nil, err
	}

	return registry.LastGCAt, nil
}

// NeedsGC reports whether an opportunistic GC should run given the
// registry's last_gc_at timestamp and the current time, per DESIGN.md's
// "gc ran within the last hour" rate limit.
func NeedsGC(lastGCAt *time.Time, now time.Time) bool {
	if lastGCAt == nil {
		return true
	}

	return now.Sub(*lastGCAt) >= GCRateLimit
}

func findClaim(claims []Claim, project, instance string) (Claim, bool) {
	for _, claim := range claims {
		if claim.Project == project && claim.Instance == instance {
			return claim, true
		}
	}

	return Claim{}, false
}

// gcStaleClaims splits claims into those whose worktree still exists and
// those that don't (per DESIGN.md: "stale claim GC (worktree path gone)").
func gcStaleClaims(claims []Claim) (kept []Claim, removed []Claim) {
	for _, claim := range claims {
		if _, err := os.Stat(claim.Worktree); errors.Is(err, fs.ErrNotExist) {
			removed = append(removed, claim)
			continue
		}

		kept = append(kept, claim)
	}

	return kept, removed
}

// firstFreeBlock scans [poolStart, poolEnd] in blockSize steps for the
// first block that overlaps no existing claim and contains no listening
// port.
func firstFreeBlock(claims []Claim, blockSize, poolStart, poolEnd int, listening map[int]bool) (int, bool) {
	for start := poolStart; start+blockSize-1 <= poolEnd; start += blockSize {
		if blockOverlapsClaims(claims, start, blockSize) {
			continue
		}

		if blockHasListeningPort(listening, start, blockSize) {
			continue
		}

		return start, true
	}

	return 0, false
}

func blockOverlapsClaims(claims []Claim, blockStart, blockSize int) bool {
	for _, claim := range claims {
		if claim.overlaps(blockStart, blockSize) {
			return true
		}
	}

	return false
}

func blockHasListeningPort(listening map[int]bool, blockStart, blockSize int) bool {
	for port := blockStart; port < blockStart+blockSize; port++ {
		if listening[port] {
			return true
		}
	}

	return false
}

// listeningPortsWithWarnings consults store.PortChecker, degrading to an
// empty (registry-only) result with a warning instead of failing when the
// checker errors — most commonly because lsof isn't installed. Per
// DESIGN.md: "lsof absence degrades to registry-only checking with a
// warning, not a failure."
func (store *Store) listeningPortsWithWarnings() (map[int]bool, []string) {
	checker := store.PortChecker
	if checker == nil {
		checker = LsofPortChecker{}
	}

	listening, err := checker.ListeningPorts()
	if err != nil {
		return map[int]bool{}, []string{
			fmt.Sprintf("port availability check unavailable (%s); falling back to registry-only overlap checking", err),
		}
	}

	return listening, nil
}

func (store *Store) load() (*registryFile, error) {
	data, err := os.ReadFile(store.RegistryPath)
	if errors.Is(err, fs.ErrNotExist) {
		return &registryFile{Version: registrySchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading registry %s: %w", store.RegistryPath, err)
	}

	if len(data) == 0 {
		return &registryFile{Version: registrySchemaVersion}, nil
	}

	var registry registryFile
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parsing registry %s: %w", store.RegistryPath, err)
	}

	if registry.Version == 0 {
		registry.Version = registrySchemaVersion
	}

	return &registry, nil
}

// save writes the registry via a temp file + rename so a crash mid-write
// never leaves a half-written registry.json behind.
func (store *Store) save(registry *registryFile) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding registry: %w", err)
	}

	tempPath := store.RegistryPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("writing registry %s: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, store.RegistryPath); err != nil {
		return fmt.Errorf("renaming registry into place: %w", err)
	}

	return nil
}

// withLock takes an exclusive flock on LockPath for the duration of fn, per
// DESIGN.md's "guarded by an exclusive flock on registry.lock for every
// read-modify-write" rule.
func (store *Store) withLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(store.LockPath), 0o755); err != nil {
		return fmt.Errorf("creating registry directory: %w", err)
	}

	lockFile, err := os.OpenFile(store.LockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("opening lock file %s: %w", store.LockPath, err)
	}
	defer lockFile.Close()

	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("locking %s: %w", store.LockPath, err)
	}
	defer unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)

	return fn()
}
