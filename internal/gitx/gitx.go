// Package gitx wraps the git CLI for the worktree, branch, and status
// operations the instance lifecycle needs. It never links libgit2 or any
// other git implementation — every operation shells out to the git binary,
// per DESIGN.md's toolchain contract.
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// run executes git with the given arguments inside dir and returns its
// trimmed stdout. Errors include stderr so callers get git's own
// explanation rather than a bare exit-status message.
func run(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// runStreaming executes git with the given arguments inside dir, streaming
// stdout/stderr directly rather than buffering them. Used for operations
// like worktree add/remove where git's own progress output is useful to the
// user as it happens.
func runStreaming(dir string, stdout, stderr *bytes.Buffer, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

// CurrentBranch returns the name of the branch checked out at dir, or an
// error if dir's HEAD is detached (rev-parse --abbrev-ref HEAD reports
// "HEAD" itself in that case, which we reject explicitly since a detached
// checkout has no meaningful branch for treehouse to derive an instance
// name or track from).
func CurrentBranch(dir string) (string, error) {
	branch, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}

	if branch == "HEAD" {
		return "", fmt.Errorf("gitx: %s has a detached HEAD, not a branch", dir)
	}

	return branch, nil
}

// StatusShort returns `git status --short` output for worktreePath, exactly
// as printed to a terminal, so callers (like `remove`'s dirty-tree refusal)
// can show the user precisely what's uncommitted.
func StatusShort(worktreePath string) (string, error) {
	return run(worktreePath, "status", "--short")
}

// IsDirty reports whether worktreePath has any uncommitted changes
// (modified, staged, or untracked files) per `git status --porcelain`.
func IsDirty(worktreePath string) (bool, error) {
	output, err := run(worktreePath, "status", "--porcelain")
	if err != nil {
		return false, err
	}

	return output != "", nil
}

// WorktreeRoot resolves the root directory of the git worktree containing
// dir, whether dir is the main repository or a linked worktree.
func WorktreeRoot(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}
