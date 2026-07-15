//go:build integration

// Integration test for the full instance lifecycle against
// testdata/fixture: create -> up -> exec -> down -> remove, per DESIGN.md's
// "Testing" section. It needs a real docker (or docker-compose) and is
// therefore excluded from the default `go test ./...` run; invoke with
// `go test -tags=integration ./internal/cli/...`.
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
)

// setupFixtureRepository copies testdata/fixture into a fresh temp git
// repository and commits it, so the fixture's own .treehouse.yml sits at
// the new repository's root — exactly where loadRepositoryContext expects
// to find it.
func setupFixtureRepository(t *testing.T) string {
	t.Helper()

	fixtureSource, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatalf("resolving fixture path: %v", err)
	}

	repo := t.TempDir()
	runShellCommand(t, repo, "cp", "-R", fixtureSource+"/.", repo)

	runShellCommand(t, repo, "git", "init", "--initial-branch=main", "-q")
	runShellCommand(t, repo, "git", "config", "user.email", "test@example.com")
	runShellCommand(t, repo, "git", "config", "user.name", "Test")
	runShellCommand(t, repo, "git", "add", ".")
	runShellCommand(t, repo, "git", "commit", "-q", "-m", "fixture")

	return repo
}

func runShellCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()

	command := exec.Command(name, args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}

// withWorkingDirectory changes the process's cwd for the duration of fn,
// restoring it afterward. The cli package's commands resolve context via
// os.Getwd() (matching a real invocation of the treehouse binary), so an
// in-process integration test needs to actually chdir rather than pass a
// directory parameter no production code path takes.
func withWorkingDirectory(t *testing.T, dir string, fn func()) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s): %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restoring cwd: %v", err)
		}
	}()

	fn()
}

// runTreehouse executes the cli command tree in-process (no separate
// binary needed) and returns its combined stdout+stderr.
func runTreehouse(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := NewRootCommand("integration-test")
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(args)

	err := root.Execute()

	return output.String(), err
}

func TestIntegration_FullLifecycle(t *testing.T) {
	repo := setupFixtureRepository(t)

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	instancesHome := t.TempDir()
	configContents := "instances_home: " + instancesHome + "\npool_start: 32000\npool_end: 32063\n"
	if err := os.MkdirAll(filepath.Join(configDir, "treehouse"), 0o755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "treehouse", "config.yml"), []byte(configContents), 0o644); err != nil {
		t.Fatalf("writing config.yml: %v", err)
	}

	withWorkingDirectory(t, repo, func() {
		output, err := runTreehouse(t, "create", "feature/integration-test", "--json")
		if err != nil {
			t.Fatalf("create failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, `"state": "provisioned"`) {
			t.Fatalf("create output does not report state=provisioned:\n%s", output)
		}
	})

	instances, err := instance.Discover(repo)
	if err != nil {
		t.Fatalf("Discover() returned unexpected error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("found %d instances, want 1", len(instances))
	}
	instancePath := instances[0].Path
	instanceName := instances[0].Name

	withWorkingDirectory(t, instancePath, func() {
		output, err := runTreehouse(t, "status")
		if err != nil {
			t.Fatalf("status failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, "running containers: 2") {
			t.Fatalf("status does not report 2 running containers:\n%s", output)
		}

		output, err = runTreehouse(t, "exec", "cache", "--", "redis-cli", "ping")
		if err != nil {
			t.Fatalf("exec failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, "PONG") {
			t.Fatalf("exec did not return PONG:\n%s", output)
		}

		if output, err := runTreehouse(t, "down"); err != nil {
			t.Fatalf("down failed: %v\n%s", err, output)
		}
	})

	withWorkingDirectory(t, repo, func() {
		if output, err := runTreehouse(t, "remove", instanceName, "--force"); err != nil {
			t.Fatalf("remove failed: %v\n%s", err, output)
		}
	})

	if _, err := os.Stat(instancePath); !os.IsNotExist(err) {
		t.Errorf("instance path %s still exists after remove", instancePath)
	}
}
