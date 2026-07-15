package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakePortChecker implements PortChecker with a fixed set of ports,
// standing in for lsof so allocation tests don't touch real sockets.
type fakePortChecker struct {
	ports map[int]bool
	err   error
}

func (checker fakePortChecker) ListeningPorts() (map[int]bool, error) {
	if checker.err != nil {
		return nil, checker.err
	}

	return checker.ports, nil
}

// newTestStore builds a Store rooted at a fresh temp dir with a no-op
// PortChecker (no ports listening), suitable for most tests.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	store := NewStore(t.TempDir())
	store.PortChecker = fakePortChecker{ports: map[int]bool{}}

	return store
}

// existingWorktree creates a directory under t.TempDir() to stand in for a
// live worktree that GC should not reclaim.
func existingWorktree(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "worktree")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating worktree fixture: %v", err)
	}

	return dir
}

func TestStore_ClaimAllocatesFirstFreeBlock(t *testing.T) {
	store := newTestStore(t)
	store.PoolStart = 4000
	store.PoolEnd = 4127

	worktree := existingWorktree(t)

	result, err := store.Claim(ClaimRequest{
		Project:    "myapp",
		Instance:   "feature-a",
		Repository: "/repo",
		Worktree:   worktree,
		BlockSize:  32,
	})
	if err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	if result.Claim.BlockStart != 4000 {
		t.Errorf("BlockStart = %d, want 4000", result.Claim.BlockStart)
	}
	if result.Existing {
		t.Errorf("Existing = true, want false for a first claim")
	}
}

func TestStore_ClaimNeverOverlapsExistingClaims(t *testing.T) {
	store := newTestStore(t)
	store.PoolStart = 4000
	store.PoolEnd = 4191 // exactly 6 blocks of 32

	var claims []Claim
	for i := 0; i < 6; i++ {
		worktree := existingWorktree(t)
		result, err := store.Claim(ClaimRequest{
			Project:    "myapp",
			Instance:   fmt.Sprintf("feature-%d", i),
			Repository: "/repo",
			Worktree:   worktree,
			BlockSize:  32,
		})
		if err != nil {
			t.Fatalf("Claim() #%d returned unexpected error: %v", i, err)
		}
		claims = append(claims, result.Claim)
	}

	for i := 0; i < len(claims); i++ {
		for j := i + 1; j < len(claims); j++ {
			if claims[i].overlaps(claims[j].BlockStart, claims[j].BlockSize) {
				t.Errorf("claim %+v overlaps claim %+v", claims[i], claims[j])
			}
		}
	}

	// The pool is exactly full now; one more claim must fail with
	// ErrPoolExhausted rather than silently overlapping.
	_, err := store.Claim(ClaimRequest{
		Project:    "myapp",
		Instance:   "overflow",
		Repository: "/repo",
		Worktree:   existingWorktree(t),
		BlockSize:  32,
	})
	if err != ErrPoolExhausted {
		t.Errorf("Claim() error = %v, want ErrPoolExhausted", err)
	}
}

func TestStore_ClaimSkipsListeningPorts(t *testing.T) {
	store := newTestStore(t)
	store.PoolStart = 4000
	store.PoolEnd = 4063 // two blocks of 32: [4000,4031] [4032,4063]
	// A listener inside the first block forces allocation into the second.
	store.PortChecker = fakePortChecker{ports: map[int]bool{4010: true}}

	result, err := store.Claim(ClaimRequest{
		Project:    "myapp",
		Instance:   "feature-a",
		Repository: "/repo",
		Worktree:   existingWorktree(t),
		BlockSize:  32,
	})
	if err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	if result.Claim.BlockStart != 4032 {
		t.Errorf("BlockStart = %d, want 4032 (first block skipped due to listening port)", result.Claim.BlockStart)
	}
}

func TestStore_ClaimDegradesGracefullyWhenPortCheckerFails(t *testing.T) {
	store := newTestStore(t)
	store.PoolStart = 4000
	store.PoolEnd = 4031
	store.PortChecker = fakePortChecker{err: os.ErrNotExist}

	result, err := store.Claim(ClaimRequest{
		Project:    "myapp",
		Instance:   "feature-a",
		Repository: "/repo",
		Worktree:   existingWorktree(t),
		BlockSize:  32,
	})
	if err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	if len(result.Warnings) != 1 {
		t.Fatalf("len(Warnings) = %d, want 1: %v", len(result.Warnings), result.Warnings)
	}
}

func TestStore_ClaimIsIdempotentPerProjectInstance(t *testing.T) {
	store := newTestStore(t)

	worktree := existingWorktree(t)
	request := ClaimRequest{
		Project:    "myapp",
		Instance:   "feature-a",
		Repository: "/repo",
		Worktree:   worktree,
		BlockSize:  32,
	}

	first, err := store.Claim(request)
	if err != nil {
		t.Fatalf("first Claim() returned unexpected error: %v", err)
	}

	second, err := store.Claim(request)
	if err != nil {
		t.Fatalf("second Claim() returned unexpected error: %v", err)
	}

	if !second.Existing {
		t.Errorf("Existing = false on repeat claim, want true")
	}
	if second.Claim.BlockStart != first.Claim.BlockStart {
		t.Errorf("BlockStart changed on repeat claim: %d != %d", second.Claim.BlockStart, first.Claim.BlockStart)
	}
}

func TestStore_GCReclaimsStaleClaims(t *testing.T) {
	store := newTestStore(t)

	liveWorktree := existingWorktree(t)
	staleWorktree := filepath.Join(t.TempDir(), "removed-worktree")
	// staleWorktree deliberately does not exist on disk.

	if _, err := store.Claim(ClaimRequest{
		Project: "myapp", Instance: "live", Repository: "/repo", Worktree: liveWorktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("claiming live instance: %v", err)
	}

	// Insert the stale claim directly so it predates GC without racing the
	// idempotency check in Claim (which would key off worktree, not path
	// existence).
	registry, err := store.load()
	if err != nil {
		t.Fatalf("loading registry: %v", err)
	}
	registry.Claims = append(registry.Claims, Claim{
		Project: "myapp", Instance: "stale", Repository: "/repo", Worktree: staleWorktree,
		BlockStart: 4032, BlockSize: 32, CreatedAt: time.Now(),
	})
	if err := store.save(registry); err != nil {
		t.Fatalf("saving registry: %v", err)
	}

	removed, err := store.GC()
	if err != nil {
		t.Fatalf("GC() returned unexpected error: %v", err)
	}

	if len(removed) != 1 || removed[0].Instance != "stale" {
		t.Fatalf("GC() removed = %+v, want exactly the stale claim", removed)
	}

	remaining, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Instance != "live" {
		t.Fatalf("List() after GC = %+v, want only the live claim", remaining)
	}

	lastGCAt, err := store.LastGCAt()
	if err != nil {
		t.Fatalf("LastGCAt() returned unexpected error: %v", err)
	}
	if lastGCAt == nil {
		t.Errorf("LastGCAt() = nil, want a timestamp after GC ran")
	}
}

func TestStore_ClaimAlsoGarbageCollectsStaleClaims(t *testing.T) {
	store := newTestStore(t)
	store.PoolStart = 4000
	store.PoolEnd = 4031 // exactly one block: the stale claim must be
	// reclaimed for a second claim to fit at all.

	staleWorktree := filepath.Join(t.TempDir(), "removed-worktree")
	registry, err := store.load()
	if err != nil {
		t.Fatalf("loading registry: %v", err)
	}
	registry.Claims = []Claim{{
		Project: "myapp", Instance: "stale", Repository: "/repo", Worktree: staleWorktree,
		BlockStart: 4000, BlockSize: 32, CreatedAt: time.Now(),
	}}
	if err := store.save(registry); err != nil {
		t.Fatalf("saving registry: %v", err)
	}

	result, err := store.Claim(ClaimRequest{
		Project: "myapp", Instance: "fresh", Repository: "/repo", Worktree: existingWorktree(t), BlockSize: 32,
	})
	if err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}
	if result.Claim.BlockStart != 4000 {
		t.Errorf("BlockStart = %d, want 4000 (stale claim's block reclaimed)", result.Claim.BlockStart)
	}
}

func TestStore_Release(t *testing.T) {
	store := newTestStore(t)
	worktree := existingWorktree(t)

	if _, err := store.Claim(ClaimRequest{
		Project: "myapp", Instance: "feature-a", Repository: "/repo", Worktree: worktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	if err := store.Release("myapp", "feature-a"); err != nil {
		t.Fatalf("Release() returned unexpected error: %v", err)
	}

	claims, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(claims) != 0 {
		t.Errorf("List() after Release = %+v, want empty", claims)
	}

	if err := store.Release("myapp", "feature-a"); err == nil {
		t.Errorf("Release() of an already-released claim returned no error")
	}
}

func TestNeedsGC(t *testing.T) {
	now := time.Now()

	if !NeedsGC(nil, now) {
		t.Errorf("NeedsGC(nil, now) = false, want true")
	}

	recent := now.Add(-1 * time.Minute)
	if NeedsGC(&recent, now) {
		t.Errorf("NeedsGC(1 minute ago, now) = true, want false")
	}

	old := now.Add(-2 * time.Hour)
	if !NeedsGC(&old, now) {
		t.Errorf("NeedsGC(2 hours ago, now) = false, want true")
	}
}

func TestStore_ConcurrentClaimsProduceDisjointBlocks(t *testing.T) {
	store := newTestStore(t)
	store.PoolStart = 4000
	store.PoolEnd = 4000 + 32*20 - 1 // 20 blocks of 32, room for every goroutine

	const goroutineCount = 20

	var waitGroup sync.WaitGroup
	results := make([]Claim, goroutineCount)
	errs := make([]error, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()

			result, err := store.Claim(ClaimRequest{
				Project:    "myapp",
				Instance:   fmt.Sprintf("instance-%d", index),
				Repository: "/repo",
				Worktree:   existingWorktree(t),
				BlockSize:  32,
			})
			if err != nil {
				errs[index] = err
				return
			}
			results[index] = result.Claim
		}(i)
	}

	waitGroup.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: Claim() returned unexpected error: %v", i, err)
		}
	}

	for i := 0; i < goroutineCount; i++ {
		for j := i + 1; j < goroutineCount; j++ {
			if results[i].overlaps(results[j].BlockStart, results[j].BlockSize) {
				t.Errorf("concurrent claims overlap: %+v and %+v", results[i], results[j])
			}
		}
	}

	claims, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(claims) != goroutineCount {
		t.Errorf("len(List()) = %d, want %d (no claims lost to a lock race)", len(claims), goroutineCount)
	}
}
