package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/geocodio/treehouse/internal/gitx"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

// setupPruneTestInstance creates a real git repository with a linked
// worktree, an instance env file inside it, and a matching registry claim,
// so runPrune can be exercised end-to-end (dirty-tree detection needs a
// real git worktree, not a bare temp directory).
func setupPruneTestInstance(t *testing.T, mainRepository, instanceName string) (worktreePath string, claim registry.Claim) {
	t.Helper()

	branch := "feature/" + instanceName
	command := exec.Command("git", "branch", branch)
	command.Dir = mainRepository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git branch %s: %v\n%s", branch, err, output)
	}

	worktreePath = filepath.Join(t.TempDir(), instanceName)
	if err := gitx.WorktreeAdd(mainRepository, worktreePath, branch, false); err != nil {
		t.Fatalf("WorktreeAdd() returned unexpected error: %v", err)
	}

	inst := instance.New(instance.NewParams{
		Project: "myapp", Name: instanceName, Branch: branch,
		MainRepository: mainRepository, Path: worktreePath, BlockStart: 4000,
		Ports: map[string]int{"HTTP_PORT": 4000},
	})
	if err := inst.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	return worktreePath, registry.Claim{
		Project: "myapp", Instance: instanceName, Repository: mainRepository,
		Worktree: worktreePath, BlockStart: 4000, BlockSize: 32,
	}
}

func TestRunPrune_DirtyWorktreeIsSkippedEvenWhenIdle(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	mainRepository := initIdleTestRepository(t)
	worktreePath, claim := setupPruneTestInstance(t, mainRepository, "dirty-instance")

	if err := os.WriteFile(filepath.Join(worktreePath, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("writing untracked file: %v", err)
	}

	store, err := registry.Open()
	if err != nil {
		t.Fatalf("registry.Open() returned unexpected error: %v", err)
	}
	if _, err := store.Claim(registry.ClaimRequest{
		Project: claim.Project, Instance: claim.Instance, Repository: claim.Repository,
		Worktree: claim.Worktree, BlockSize: claim.BlockSize,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	// --days 0 makes the instance idle immediately, isolating the test from
	// the dirty-tree check rather than timing.
	var output bytes.Buffer
	command := NewRootCommand("test")
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"prune", "--days", "0", "--force"})
	if err := command.Execute(); err != nil {
		t.Fatalf("prune failed: %v\n%s", err, output.String())
	}

	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("dirty worktree %s was removed by prune, want it left alone: %v", worktreePath, err)
	}

	remaining, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("List() after prune = %+v, want the dirty instance's claim untouched", remaining)
	}
}

func TestRunPrune_CleanIdleInstanceIsRemovedWithForce(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	mainRepository := initIdleTestRepository(t)
	worktreePath, claim := setupPruneTestInstance(t, mainRepository, "clean-instance")

	store, err := registry.Open()
	if err != nil {
		t.Fatalf("registry.Open() returned unexpected error: %v", err)
	}
	if _, err := store.Claim(registry.ClaimRequest{
		Project: claim.Project, Instance: claim.Instance, Repository: claim.Repository,
		Worktree: claim.Worktree, BlockSize: claim.BlockSize,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var output bytes.Buffer
	command := NewRootCommand("test")
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"prune", "--days", "0", "--force"})
	if err := command.Execute(); err != nil {
		t.Fatalf("prune failed: %v\n%s", err, output.String())
	}

	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("clean idle worktree %s still exists after prune --force", worktreePath)
	}

	remaining, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("List() after prune = %+v, want the claim released", remaining)
	}
}

func TestRunPrune_WithoutForceAndNoTTYExitsWouldPrompt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	mainRepository := initIdleTestRepository(t)
	_, claim := setupPruneTestInstance(t, mainRepository, "prompt-instance")

	store, err := registry.Open()
	if err != nil {
		t.Fatalf("registry.Open() returned unexpected error: %v", err)
	}
	if _, err := store.Claim(registry.ClaimRequest{
		Project: claim.Project, Instance: claim.Instance, Repository: claim.Repository,
		Worktree: claim.Worktree, BlockSize: claim.BlockSize,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var output bytes.Buffer
	command := NewRootCommand("test")
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"prune", "--days", "0"})
	err = command.Execute()
	if err == nil {
		t.Fatalf("prune without --force succeeded in a non-interactive test process, want ExitWouldPrompt")
	}
	if ExitCodeFor(err) != ExitWouldPrompt {
		t.Errorf("ExitCodeFor(err) = %v, want ExitWouldPrompt", ExitCodeFor(err))
	}
}

func TestRunPrune_DryRunRemovesNothing(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	mainRepository := initIdleTestRepository(t)
	worktreePath, claim := setupPruneTestInstance(t, mainRepository, "dry-run-instance")

	store, err := registry.Open()
	if err != nil {
		t.Fatalf("registry.Open() returned unexpected error: %v", err)
	}
	if _, err := store.Claim(registry.ClaimRequest{
		Project: claim.Project, Instance: claim.Instance, Repository: claim.Repository,
		Worktree: claim.Worktree, BlockSize: claim.BlockSize,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var output bytes.Buffer
	command := NewRootCommand("test")
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"prune", "--days", "0", "--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatalf("prune --dry-run failed: %v\n%s", err, output.String())
	}

	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree %s was removed by --dry-run, want it left alone: %v", worktreePath, err)
	}

	remaining, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("List() after --dry-run = %+v, want the claim untouched", remaining)
	}
}

// TestRunPrune_RecentInstanceIsNeverTouched guards against an
// implementation that ignores the --days threshold entirely.
func TestRunPrune_RecentInstanceIsNeverTouched(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	mainRepository := initIdleTestRepository(t)
	worktreePath, claim := setupPruneTestInstance(t, mainRepository, "recent-instance")

	store, err := registry.Open()
	if err != nil {
		t.Fatalf("registry.Open() returned unexpected error: %v", err)
	}
	if _, err := store.Claim(registry.ClaimRequest{
		Project: claim.Project, Instance: claim.Instance, Repository: claim.Repository,
		Worktree: claim.Worktree, BlockSize: claim.BlockSize,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var output bytes.Buffer
	command := NewRootCommand("test")
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"prune", "--days", "30", "--force"})
	if err := command.Execute(); err != nil {
		t.Fatalf("prune failed: %v\n%s", err, output.String())
	}

	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("recently-active worktree %s was removed, want it left alone: %v", worktreePath, err)
	}
}
