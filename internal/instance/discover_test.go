package instance

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestRepository(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init", "--initial-branch=main", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "initial commit")

	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestDiscover_FindsOnlyWorktreesWithEnvFiles(t *testing.T) {
	repo := initTestRepository(t)

	runGit(t, repo, "branch", "feature-a")
	worktreePath := filepath.Join(t.TempDir(), "feature-a")
	runGit(t, repo, "worktree", "add", worktreePath, "feature-a")

	// A second linked worktree with no .treehouse.env must be ignored: not
	// every worktree is a treehouse instance.
	runGit(t, repo, "branch", "plain-worktree")
	plainWorktreePath := filepath.Join(t.TempDir(), "plain-worktree")
	runGit(t, repo, "worktree", "add", plainWorktreePath, "plain-worktree")

	instanceForFeatureA := New(NewParams{
		Project:        "myapp",
		Name:           "feature-a",
		Branch:         "feature-a",
		MainRepository: repo,
		Path:           worktreePath,
		BlockStart:     4640,
		Ports:          map[string]int{"HTTP_PORT": 4640},
	})
	if err := instanceForFeatureA.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	found, err := Discover(repo)
	if err != nil {
		t.Fatalf("Discover() returned unexpected error: %v", err)
	}

	if len(found) != 1 {
		t.Fatalf("Discover() found %d instances, want 1: %+v", len(found), found)
	}
	if found[0].Name != "feature-a" {
		t.Errorf("Discover()[0].Name = %q, want feature-a", found[0].Name)
	}
}

func TestFindEnvFileUpwards(t *testing.T) {
	root := t.TempDir()
	instance := New(NewParams{
		Project: "myapp", Name: "feature-a", Branch: "feature-a",
		MainRepository: "/repo", Path: root, BlockStart: 4000,
		Ports: map[string]int{"HTTP_PORT": 4000},
	})
	if err := instance.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	nested := filepath.Join(root, "app", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("creating nested dir: %v", err)
	}

	path, found, err := FindEnvFileUpwards(nested)
	if err != nil {
		t.Fatalf("FindEnvFileUpwards() returned unexpected error: %v", err)
	}
	if !found {
		t.Fatalf("FindEnvFileUpwards() found = false, want true")
	}
	if path != filepath.Join(root, EnvFileName) {
		t.Errorf("FindEnvFileUpwards() path = %q, want %q", path, filepath.Join(root, EnvFileName))
	}

	_, found, err = FindEnvFileUpwards(t.TempDir())
	if err != nil {
		t.Fatalf("FindEnvFileUpwards() returned unexpected error: %v", err)
	}
	if found {
		t.Errorf("FindEnvFileUpwards() found = true for an unrelated directory, want false")
	}
}
