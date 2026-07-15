package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/geocodio/treehouse/internal/registry"
)

// initIdleTestRepository creates a throwaway git repository (with a real
// HEAD reflog) and returns its path, so instanceActivity's git-signal path
// can be exercised without a real docker daemon.
func initIdleTestRepository(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		command := exec.Command("git", args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	// Matches DESIGN.md's "adopting repos gitignore .treehouse.env"
	// convention, so a worktree with a freshly written env file (as every
	// real instance has) isn't reported dirty by prune's IsDirty check.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".treehouse.env\n"), 0o644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md", ".gitignore"},
		{"commit", "-q", "-m", "initial commit"},
	} {
		command := exec.Command("git", args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}

	return dir
}

func TestIsIdleBeyond_RecentReflogIsNotIdle(t *testing.T) {
	repo := initIdleTestRepository(t)
	claim := registry.Claim{Project: "myapp", Instance: "feature-a", Worktree: repo}

	idle, _, err := isIdleBeyond(claim, 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("isIdleBeyond() returned unexpected error: %v", err)
	}
	if idle {
		t.Errorf("isIdleBeyond() = true for a just-committed repository, want false")
	}
}

func TestIsIdleBeyond_OldActivityIsIdle(t *testing.T) {
	repo := initIdleTestRepository(t)
	claim := registry.Claim{Project: "myapp", Instance: "feature-a", Worktree: repo}

	// A threshold of zero makes "just now" already past it, standing in for
	// "activity happened a long time ago" without needing to backdate any
	// real file mtimes.
	idle, reason, err := isIdleBeyond(claim, 0, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("isIdleBeyond() returned unexpected error: %v", err)
	}
	if !idle {
		t.Errorf("isIdleBeyond() = false, want true when now is far past the activity signal")
	}
	if reason == "" {
		t.Errorf("isIdleBeyond() reason is empty, want a description of the idle signal")
	}
}

func TestInstanceActivity_PrefersEnvFileMTimeWhenNewest(t *testing.T) {
	repo := initIdleTestRepository(t)

	// .treehouse.env doesn't need real instance contents for this signal —
	// only its own mtime matters.
	envPath := filepath.Join(repo, ".treehouse.env")
	if err := os.WriteFile(envPath, []byte("# managed by treehouse\n"), 0o644); err != nil {
		t.Fatalf("writing .treehouse.env: %v", err)
	}

	claim := registry.Claim{Project: "myapp", Instance: "feature-a", Worktree: repo}

	newest, source, err := instanceActivity(claim)
	if err != nil {
		t.Fatalf("instanceActivity() returned unexpected error: %v", err)
	}
	if source != ".treehouse.env" {
		t.Errorf("source = %q, want .treehouse.env (its mtime is newest: written after the repo's commit)", source)
	}
	if newest.IsZero() {
		t.Errorf("instanceActivity() = zero time, want a real mtime")
	}
}
