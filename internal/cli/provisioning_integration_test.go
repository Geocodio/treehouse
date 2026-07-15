//go:build integration

// Integration test for `create --no-start` followed by `treehouse
// provision`, needing a real docker (or docker-compose). Invoke with
// `go test -tags=integration ./internal/cli/...`.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
)

// TestIntegration_NoStartThenProvisionCompletesStateMachine guards
// DESIGN.md's corrected provisioning order end to end: `create --no-start`
// must stop at deps-cloned without ever running the hook (the fixture hook
// fails outright if the stack isn't up yet — see
// testdata/fixture/scripts/treehouse-post-create.sh), and a later
// `treehouse provision` (with no --retry) must bring the stack up and
// complete the hook from there.
func TestIntegration_NoStartThenProvisionCompletesStateMachine(t *testing.T) {
	repo := setupFixtureRepository(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	withWorkingDirectory(t, repo, func() {
		output, err := runTreehouse(t, "create", "feature/no-start-test", "--no-start", "--json")
		if err != nil {
			t.Fatalf("create --no-start failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, `"state": "deps-cloned"`) {
			t.Fatalf("create --no-start output does not report state=deps-cloned:\n%s", output)
		}
	})

	instances, err := instance.Discover(repo)
	if err != nil {
		t.Fatalf("Discover() returned unexpected error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("found %d instances, want 1", len(instances))
	}
	created := instances[0]
	if created.State != instance.StateDepsCloned {
		t.Fatalf("instance state = %q, want deps-cloned", created.State)
	}

	withWorkingDirectory(t, created.Path, func() {
		// No --retry: per DESIGN.md, provision must complete a
		// --no-start instance without it, since deps-cloned was never
		// fully provisioned to begin with.
		output, err := runTreehouse(t, "provision")
		if err != nil {
			t.Fatalf("provision failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, "provisioned") {
			t.Fatalf("provision output does not confirm completion:\n%s", output)
		}
	})

	reloaded, err := instance.LoadEnvFile(filepath.Join(created.Path, instance.EnvFileName))
	if err != nil {
		t.Fatalf("LoadEnvFile() returned unexpected error: %v", err)
	}
	if reloaded.State != instance.StateProvisioned {
		t.Errorf("instance state after provision = %q, want provisioned", reloaded.State)
	}

	withWorkingDirectory(t, repo, func() {
		if output, err := runTreehouse(t, "remove", created.Name, "--force"); err != nil {
			t.Fatalf("remove failed: %v\n%s", err, output)
		}
	})

	if _, err := os.Stat(created.Path); !os.IsNotExist(err) {
		t.Errorf("instance path %s still exists after remove", created.Path)
	}
}
