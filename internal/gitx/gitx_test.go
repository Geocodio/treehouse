package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepository creates a throwaway git repository with a single
// commit on its default branch, suitable for exercising real gitx
// operations against a real git binary (assumed present per DESIGN.md's
// toolchain contract, so no build tag is needed here).
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

func TestCurrentBranch(t *testing.T) {
	repo := initTestRepository(t)

	branch, err := CurrentBranch(repo)
	if err != nil {
		t.Fatalf("CurrentBranch() returned unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch() = %q, want main", branch)
	}
}

func TestBranchExistsLocal(t *testing.T) {
	repo := initTestRepository(t)

	if exists, err := BranchExistsLocal(repo, "does-not-exist"); err != nil || exists {
		t.Errorf("BranchExistsLocal(does-not-exist) = %v, %v, want false, nil", exists, err)
	}

	runGit(t, repo, "branch", "feature-a")

	if exists, err := BranchExistsLocal(repo, "feature-a"); err != nil || !exists {
		t.Errorf("BranchExistsLocal(feature-a) = %v, %v, want true, nil", exists, err)
	}
}

func TestCreateBranch_FromCurrentWhenNoRemote(t *testing.T) {
	repo := initTestRepository(t)

	if err := CreateBranch(repo, "feature-a"); err != nil {
		t.Fatalf("CreateBranch() returned unexpected error: %v", err)
	}

	exists, err := BranchExistsLocal(repo, "feature-a")
	if err != nil {
		t.Fatalf("BranchExistsLocal() returned unexpected error: %v", err)
	}
	if !exists {
		t.Errorf("BranchExistsLocal(feature-a) = false after CreateBranch(), want true")
	}
}

func TestFindMainRepository(t *testing.T) {
	repo := initTestRepository(t)
	runGit(t, repo, "branch", "feature-a")

	worktreePath := filepath.Join(t.TempDir(), "feature-a")
	runGit(t, repo, "worktree", "add", worktreePath, "feature-a")

	// Resolving from inside the linked worktree must still find the main
	// repository, not the worktree itself.
	main, err := FindMainRepository(worktreePath)
	if err != nil {
		t.Fatalf("FindMainRepository() returned unexpected error: %v", err)
	}

	// Resolve symlinks on both sides: on macOS, t.TempDir() lives under
	// /var which is itself a symlink to /private/var, and git reports the
	// resolved path.
	wantMain, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(repo): %v", err)
	}
	gotMain, err := filepath.EvalSymlinks(main)
	if err != nil {
		t.Fatalf("EvalSymlinks(main): %v", err)
	}
	if gotMain != wantMain {
		t.Errorf("FindMainRepository() = %q, want %q", gotMain, wantMain)
	}
}

func TestWorktreeAddAndRemove(t *testing.T) {
	repo := initTestRepository(t)
	runGit(t, repo, "branch", "feature-a")

	worktreePath := filepath.Join(t.TempDir(), "feature-a")

	if err := WorktreeAdd(repo, worktreePath, "feature-a", false); err != nil {
		t.Fatalf("WorktreeAdd() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("worktree path does not exist after WorktreeAdd(): %v", err)
	}

	dirty, err := IsDirty(worktreePath)
	if err != nil {
		t.Fatalf("IsDirty() returned unexpected error: %v", err)
	}
	if dirty {
		t.Errorf("IsDirty() = true for a freshly added worktree, want false")
	}

	if err := WorktreeRemove(repo, worktreePath, false); err != nil {
		t.Fatalf("WorktreeRemove() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("worktree path still exists after WorktreeRemove()")
	}
}

func TestWorktreeRemove_RefusesDirtyWithoutForce(t *testing.T) {
	repo := initTestRepository(t)
	runGit(t, repo, "branch", "feature-a")

	worktreePath := filepath.Join(t.TempDir(), "feature-a")
	if err := WorktreeAdd(repo, worktreePath, "feature-a", false); err != nil {
		t.Fatalf("WorktreeAdd() returned unexpected error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("writing untracked file: %v", err)
	}

	dirty, err := IsDirty(worktreePath)
	if err != nil {
		t.Fatalf("IsDirty() returned unexpected error: %v", err)
	}
	if !dirty {
		t.Fatalf("IsDirty() = false with an untracked file present, want true")
	}

	if err := WorktreeRemove(repo, worktreePath, false); err == nil {
		t.Errorf("WorktreeRemove() without --force succeeded on a dirty worktree, want refusal")
	}

	if err := WorktreeRemove(repo, worktreePath, true); err != nil {
		t.Errorf("WorktreeRemove() with --force returned unexpected error: %v", err)
	}
}

func TestGitDir_LinkedWorktreeResolvesToItsOwnGitDirUnderMain(t *testing.T) {
	repo := initTestRepository(t)
	runGit(t, repo, "branch", "feature-a")

	worktreePath := filepath.Join(t.TempDir(), "feature-a")
	if err := WorktreeAdd(repo, worktreePath, "feature-a", false); err != nil {
		t.Fatalf("WorktreeAdd() returned unexpected error: %v", err)
	}

	gitDir, err := GitDir(worktreePath)
	if err != nil {
		t.Fatalf("GitDir() returned unexpected error: %v", err)
	}

	if _, err := os.Stat(gitDir); err != nil {
		t.Fatalf("GitDir() = %q does not exist: %v", gitDir, err)
	}
	if filepath.Base(filepath.Dir(gitDir)) != "worktrees" {
		t.Errorf("GitDir() = %q, want a path under a \"worktrees\" directory", gitDir)
	}
}

func TestReflogHeadMTime_FindsRealReflogFile(t *testing.T) {
	repo := initTestRepository(t)

	mtime, ok, err := ReflogHeadMTime(repo)
	if err != nil {
		t.Fatalf("ReflogHeadMTime() returned unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("ReflogHeadMTime() ok = false, want true for a repository with commits")
	}
	if mtime.IsZero() {
		t.Errorf("ReflogHeadMTime() = zero time, want a real mtime")
	}
}

func TestReflogHeadMTime_NonGitDirectoryReturnsError(t *testing.T) {
	// GitDir shells out to `git rev-parse --git-dir`, which fails outright
	// for a directory that isn't a git repository at all; callers (gc's
	// idle detection) already treat this error the same as "signal
	// unavailable", same as a missing reflog file.
	if _, _, err := ReflogHeadMTime(t.TempDir()); err == nil {
		t.Errorf("ReflogHeadMTime() returned nil error for a non-git directory, want an error")
	}
}
