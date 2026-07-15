package gitx

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// WorktreeEntry is one block of `git worktree list --porcelain` output: a
// single worktree's path, HEAD commit, and branch (or detached-HEAD state).
type WorktreeEntry struct {
	Path     string
	Head     string
	Branch   string // refs/heads/<name>; empty when Detached is true
	Detached bool
	Locked   bool
	Prunable bool
}

// ListWorktrees runs `git worktree list --porcelain` inside dir and parses
// the result. The first entry returned is always the main worktree, per
// git's own documented ordering.
func ListWorktrees(dir string) ([]WorktreeEntry, error) {
	output, err := run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	return parseWorktreePorcelain([]byte(output)), nil
}

// FindMainRepository returns the path of the repository's main worktree
// (the first entry `git worktree list --porcelain` reports), regardless of
// whether dir is itself the main worktree or a linked one.
func FindMainRepository(dir string) (string, error) {
	entries, err := ListWorktrees(dir)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("gitx: %s reported no worktrees", dir)
	}

	return entries[0].Path, nil
}

// parseWorktreePorcelain parses `git worktree list --porcelain` output.
// Entries are separated by a blank line; within an entry, each line is
// either a bare keyword (detached, locked, prunable) or a "keyword value"
// pair (worktree, HEAD, branch). This is a pure function so the format can
// be exercised with fixture strings independent of a real git binary.
func parseWorktreePorcelain(output []byte) []WorktreeEntry {
	var entries []WorktreeEntry
	var current *WorktreeEntry

	flush := func() {
		if current != nil {
			entries = append(entries, *current)
			current = nil
		}
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}

		key, value, _ := strings.Cut(line, " ")

		switch key {
		case "worktree":
			flush()
			current = &WorktreeEntry{Path: value}
		case "HEAD":
			if current != nil {
				current.Head = value
			}
		case "branch":
			if current != nil {
				current.Branch = value
			}
		case "detached":
			if current != nil {
				current.Detached = true
			}
		case "locked":
			if current != nil {
				current.Locked = true
			}
		case "prunable":
			if current != nil {
				current.Prunable = true
			}
		}
	}
	flush()

	return entries
}

// WorktreeAdd creates a worktree at path checked out to branch. force is
// forwarded as `--force` only when explicitly requested by the caller —
// never pass it reflexively, per DESIGN.md's worktree operation rules.
func WorktreeAdd(repositoryDir, path, branch string, force bool) error {
	args := []string{"worktree", "add"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path, branch)

	var stdout, stderr bytes.Buffer
	if err := runStreaming(repositoryDir, &stdout, &stderr, args...); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git worktree add: %s", message)
	}

	return nil
}

// WorktreeRemove removes the worktree at path. force is forwarded as
// `--force` only when explicitly requested by the caller: DESIGN.md
// requires `remove`/`prune` to attempt this without --force first, showing
// `git status --short` and requiring an explicit --force on refusal.
func WorktreeRemove(repositoryDir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)

	var stdout, stderr bytes.Buffer
	if err := runStreaming(repositoryDir, &stdout, &stderr, args...); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git worktree remove: %s", message)
	}

	return nil
}
