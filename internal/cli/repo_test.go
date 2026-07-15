package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGitCommand runs git inside dir, failing the test on any error. It
// deliberately doesn't share a name with any git helper in the (build-tag
// gated) integration test files, since both compile together under
// `-tags=integration`.
func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

// initRepositoryWithoutManifest creates a throwaway git repository on
// "main" with a single commit and no .treehouse.yml at all, so tests can
// add the manifest only on a branch/worktree of their choosing.
func initRepositoryWithoutManifest(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGitCommand(t, dir, "init", "--initial-branch=main", "-q")
	runGitCommand(t, dir, "config", "user.email", "test@example.com")
	runGitCommand(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	runGitCommand(t, dir, "add", "README.md")
	runGitCommand(t, dir, "commit", "-q", "-m", "initial commit")

	return dir
}

// evalSymlinksOrFatal resolves path's symlinks (macOS's t.TempDir() lives
// under /var, itself a symlink to /private/var, and git reports the
// resolved path) so path comparisons against gitx output are exact.
func evalSymlinksOrFatal(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", path, err)
	}
	return resolved
}

// TestLoadRepositoryContext_ManifestFromSecondaryWorktree guards BUG 1's
// fix: a real adoption dry-run where a feature branch adds .treehouse.yml,
// checked out into a `git worktree add` worktree, while the main checkout
// stays on a branch that never touched the manifest at all (the ordinary
// state of any repository mid-adoption). `doctor`/`create` must resolve
// the manifest from that worktree's own root, never fail with "no
// manifest" just because the main checkout doesn't have one.
func TestLoadRepositoryContext_ManifestFromSecondaryWorktree(t *testing.T) {
	main := initRepositoryWithoutManifest(t)

	worktreePath := filepath.Join(t.TempDir(), "feature-add-manifest")
	runGitCommand(t, main, "worktree", "add", "-b", "feature/add-manifest", worktreePath)

	manifestContents := "version: 1\nproject: adopted\n"
	if err := os.WriteFile(filepath.Join(worktreePath, manifestFileName), []byte(manifestContents), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	runGitCommand(t, worktreePath, "add", manifestFileName)
	runGitCommand(t, worktreePath, "commit", "-q", "-m", "add treehouse manifest")

	// Sanity-check the scenario itself: the main checkout truly has no
	// manifest, so a naive main-repository resolution would reproduce the
	// exact failure hit during the real adoption dry-run.
	if _, err := os.Stat(filepath.Join(main, manifestFileName)); !os.IsNotExist(err) {
		t.Fatalf("test setup: main checkout unexpectedly has a manifest")
	}

	repo, err := loadRepositoryContext(worktreePath)
	if err != nil {
		t.Fatalf("loadRepositoryContext() returned unexpected error: %v", err)
	}

	if repo.Manifest.Project != "adopted" {
		t.Errorf("Manifest.Project = %q, want adopted", repo.Manifest.Project)
	}

	wantCheckoutRoot := evalSymlinksOrFatal(t, worktreePath)
	if gotCheckoutRoot := evalSymlinksOrFatal(t, repo.CheckoutRoot); gotCheckoutRoot != wantCheckoutRoot {
		t.Errorf("CheckoutRoot = %q, want %q", gotCheckoutRoot, wantCheckoutRoot)
	}

	wantMainRepository := evalSymlinksOrFatal(t, main)
	if gotMainRepository := evalSymlinksOrFatal(t, repo.MainRepository); gotMainRepository != wantMainRepository {
		t.Errorf("MainRepository = %q, want %q", gotMainRepository, wantMainRepository)
	}
}

// TestLoadRepositoryContext_MainRepositoryWithoutManifestStillFails checks
// the fix didn't accidentally widen manifest resolution to "anywhere in
// the repository": invoked directly from the main checkout, which alone
// lacks the manifest, loadRepositoryContext must still report "not a
// treehouse-enabled repository" rather than finding some other worktree's
// copy.
func TestLoadRepositoryContext_MainRepositoryWithoutManifestStillFails(t *testing.T) {
	main := initRepositoryWithoutManifest(t)

	worktreePath := filepath.Join(t.TempDir(), "feature-add-manifest")
	runGitCommand(t, main, "worktree", "add", "-b", "feature/add-manifest", worktreePath)

	manifestContents := "version: 1\nproject: adopted\n"
	if err := os.WriteFile(filepath.Join(worktreePath, manifestFileName), []byte(manifestContents), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	runGitCommand(t, worktreePath, "add", manifestFileName)
	runGitCommand(t, worktreePath, "commit", "-q", "-m", "add treehouse manifest")

	if _, err := loadRepositoryContext(main); err == nil {
		t.Fatalf("loadRepositoryContext(main) succeeded even though only a secondary worktree has the manifest, want an error")
	}
}

// TestEnsureBranch_BasesNewBranchOnInvokingCheckoutsCurrentBranch guards
// the other half of BUG 1: `create`'s new branch must be based on the
// invoking checkout's own current branch, not on whatever branch the
// repository's main worktree happens to have checked out. Before the fix,
// ensureBranch always ran `git branch` inside the main repository, so a
// worktree parked on a different branch than main silently branched off
// main's tip instead of its own.
func TestEnsureBranch_BasesNewBranchOnInvokingCheckoutsCurrentBranch(t *testing.T) {
	main := initRepositoryWithoutManifest(t)

	secondaryPath := filepath.Join(t.TempDir(), "secondary")
	runGitCommand(t, main, "worktree", "add", "-b", "existing-branch", secondaryPath)

	// A commit that exists only on existing-branch (checked out in
	// secondaryPath), never on main.
	if err := os.WriteFile(filepath.Join(secondaryPath, "secondary-only.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing secondary-only.txt: %v", err)
	}
	runGitCommand(t, secondaryPath, "add", "secondary-only.txt")
	runGitCommand(t, secondaryPath, "commit", "-q", "-m", "secondary-only commit")

	if err := ensureBranch(secondaryPath, "new-branch"); err != nil {
		t.Fatalf("ensureBranch() returned unexpected error: %v", err)
	}

	// new-branch must contain secondary-only.txt (proving it was based on
	// secondaryPath's HEAD), not fail to resolve it (which is what would
	// happen if it had been based on main's HEAD instead).
	command := exec.Command("git", "show", "new-branch:secondary-only.txt")
	command.Dir = main
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("new-branch does not contain secondary-only.txt (based on the wrong branch): %v\n%s", err, output)
	}
	if string(output) != "x" {
		t.Errorf("new-branch:secondary-only.txt = %q, want x", output)
	}
}
