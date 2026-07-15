//go:build integration

// Integration test for `treehouse gc` stopping a real running instance,
// needing a real docker (or docker-compose). Invoke with
// `go test -tags=integration ./internal/cli/...`.
package cli

import (
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
)

func TestIntegration_GCStopsIdleRunningInstance(t *testing.T) {
	repo := setupFixtureRepository(t)

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	withWorkingDirectory(t, repo, func() {
		if output, err := runTreehouse(t, "create", "feature/gc-integration-test"); err != nil {
			t.Fatalf("create failed: %v\n%s", err, output)
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

	count, err := compose.RunningContainerCount(created.ComposeProject)
	if err != nil {
		t.Fatalf("RunningContainerCount() returned unexpected error: %v", err)
	}
	if count == 0 {
		t.Fatalf("instance has 0 running containers right after create, want > 0")
	}

	withWorkingDirectory(t, repo, func() {
		// --idle-hours 0 makes "just created" already past the threshold,
		// standing in for a real idle instance without needing to backdate
		// any files.
		output, err := runTreehouse(t, "gc", "--idle-hours", "0")
		if err != nil {
			t.Fatalf("gc failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, created.Name) {
			t.Errorf("gc output does not mention the stopped instance %q:\n%s", created.Name, output)
		}
	})

	count, err = compose.RunningContainerCount(created.ComposeProject)
	if err != nil {
		t.Fatalf("RunningContainerCount() returned unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("instance still has %d running containers after gc, want 0", count)
	}

	withWorkingDirectory(t, repo, func() {
		if output, err := runTreehouse(t, "remove", created.Name, "--force"); err != nil {
			t.Fatalf("remove failed: %v\n%s", err, output)
		}
	})
}
