package instance

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// EnvFileName is the file name (not path) of the instance env file, written
// at the root of every instance's worktree.
const EnvFileName = ".treehouse.env"

// managedHeader is written as the file's first line, per DESIGN.md's exact
// ".treehouse.env" contract: it is both a docker-compose env file and the
// dispatch marker adopting repos use to route between `docker compose` and
// `treehouse compose`.
const managedHeader = "# Managed by treehouse — do not edit. Regenerate with: treehouse provision"

const lockHashPrefix = "TREEHOUSE_LOCK_HASH_"

// reservedKeys are the fixed .treehouse.env keys that are never themselves
// port variables, so EnvFilePath's reader can tell a manifest-declared port
// variable apart from treehouse's own bookkeeping keys.
var reservedKeys = map[string]bool{
	"COMPOSE_PROJECT_NAME":      true,
	"TREEHOUSE_PROJECT":         true,
	"TREEHOUSE_INSTANCE":        true,
	"TREEHOUSE_BRANCH":          true,
	"TREEHOUSE_MAIN_REPOSITORY": true,
	"TREEHOUSE_BLOCK":           true,
	"TREEHOUSE_STATE":           true,
}

// EnvFilePath returns the path to this instance's .treehouse.env.
func (instance *Instance) EnvFilePath() string {
	return filepath.Join(instance.Path, EnvFileName)
}

// Save writes .treehouse.env atomically (temp file + rename), so a crash
// mid-write never leaves a partially written env file for docker compose or
// a hook script to read.
func (instance *Instance) Save() error {
	contents := instance.render()

	envFilePath := instance.EnvFilePath()
	tempPath := envFilePath + ".tmp"

	if err := os.WriteFile(tempPath, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, envFilePath); err != nil {
		return fmt.Errorf("renaming %s into place: %w", envFilePath, err)
	}

	return nil
}

// EnvPairs returns every .treehouse.env entry as "KEY=VALUE" strings,
// suitable for appending to an exec.Cmd's Env so a child process (like the
// provisioning hook) sees the instance's full environment.
func (instance *Instance) EnvPairs() []string {
	pairs := []string{
		"COMPOSE_PROJECT_NAME=" + instance.ComposeProject,
		"TREEHOUSE_PROJECT=" + instance.Project,
		"TREEHOUSE_INSTANCE=" + instance.Name,
		"TREEHOUSE_BRANCH=" + instance.Branch,
		"TREEHOUSE_MAIN_REPOSITORY=" + instance.MainRepository,
		fmt.Sprintf("TREEHOUSE_BLOCK=%d", instance.BlockStart),
		"TREEHOUSE_STATE=" + string(instance.State),
	}

	for _, lockfile := range sortedKeys(instance.LockHashes) {
		pairs = append(pairs, lockHashPrefix+lockfile+"="+instance.LockHashes[lockfile])
	}

	for _, name := range sortedKeys(instance.Ports) {
		pairs = append(pairs, fmt.Sprintf("%s=%d", name, instance.Ports[name]))
	}

	return pairs
}

// render produces .treehouse.env's exact contents. Lock hashes and ports
// are sorted by key so the file is byte-for-byte deterministic across runs
// (the manifest's ports.env is an unordered map, so without sorting, key
// order would depend on Go's randomized map iteration).
func (instance *Instance) render() string {
	var builder strings.Builder

	builder.WriteString(managedHeader)
	builder.WriteString("\n")
	fmt.Fprintf(&builder, "COMPOSE_PROJECT_NAME=%s\n", instance.ComposeProject)
	fmt.Fprintf(&builder, "TREEHOUSE_PROJECT=%s\n", instance.Project)
	fmt.Fprintf(&builder, "TREEHOUSE_INSTANCE=%s\n", instance.Name)
	fmt.Fprintf(&builder, "TREEHOUSE_BRANCH=%s\n", instance.Branch)
	fmt.Fprintf(&builder, "TREEHOUSE_MAIN_REPOSITORY=%s\n", instance.MainRepository)
	fmt.Fprintf(&builder, "TREEHOUSE_BLOCK=%d\n", instance.BlockStart)
	fmt.Fprintf(&builder, "TREEHOUSE_STATE=%s\n", instance.State)

	for _, lockfile := range sortedKeys(instance.LockHashes) {
		fmt.Fprintf(&builder, "%s%s=%s\n", lockHashPrefix, lockfile, instance.LockHashes[lockfile])
	}

	for _, name := range sortedKeys(instance.Ports) {
		fmt.Fprintf(&builder, "%s=%d\n", name, instance.Ports[name])
	}

	return builder.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

// LoadEnvFile reads and parses the .treehouse.env at envFilePath. The
// instance's Path is set to the file's containing directory.
func LoadEnvFile(envFilePath string) (*Instance, error) {
	file, err := os.Open(envFilePath)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", envFilePath, err)
	}
	defer file.Close()

	result := &Instance{
		Path:       filepath.Dir(envFilePath),
		LockHashes: map[string]string{},
		Ports:      map[string]int{},
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		if err := result.assign(key, value); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", envFilePath, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", envFilePath, err)
	}

	return result, nil
}

func (instance *Instance) assign(key, value string) error {
	switch {
	case key == "COMPOSE_PROJECT_NAME":
		instance.ComposeProject = value
	case key == "TREEHOUSE_PROJECT":
		instance.Project = value
	case key == "TREEHOUSE_INSTANCE":
		instance.Name = value
	case key == "TREEHOUSE_BRANCH":
		instance.Branch = value
	case key == "TREEHOUSE_MAIN_REPOSITORY":
		instance.MainRepository = value
	case key == "TREEHOUSE_BLOCK":
		block, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("TREEHOUSE_BLOCK: %w", err)
		}
		instance.BlockStart = block
	case key == "TREEHOUSE_STATE":
		instance.State = State(value)
	case strings.HasPrefix(key, lockHashPrefix):
		instance.LockHashes[strings.TrimPrefix(key, lockHashPrefix)] = value
	case reservedKeys[key]:
		// Recognized-but-unhandled reserved key; nothing else to do.
	default:
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s: expected a port number, got %q", key, value)
		}
		instance.Ports[key] = port
	}

	return nil
}
