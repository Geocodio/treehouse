package gitx

import "testing"

func TestParseWorktreePorcelain(t *testing.T) {
	output := []byte(`worktree /home/user/projects/myapp
HEAD abcdef0123456789abcdef0123456789abcdef01
branch refs/heads/main

worktree /home/user/treehouses/myapp/my-feature
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature/my-feature

worktree /home/user/treehouses/myapp/detached-thing
HEAD 2222222222222222222222222222222222222222
detached

worktree /home/user/treehouses/myapp/locked-thing
HEAD 3333333333333333333333333333333333333333
branch refs/heads/locked-branch
locked

worktree /home/user/treehouses/myapp/prunable-thing
HEAD 4444444444444444444444444444444444444444
branch refs/heads/prunable-branch
prunable
`)

	entries := parseWorktreePorcelain(output)

	if len(entries) != 5 {
		t.Fatalf("len(entries) = %d, want 5", len(entries))
	}

	main := entries[0]
	if main.Path != "/home/user/projects/myapp" {
		t.Errorf("entries[0].Path = %q, want main repository path", main.Path)
	}
	if main.Branch != "refs/heads/main" {
		t.Errorf("entries[0].Branch = %q, want refs/heads/main", main.Branch)
	}
	if main.Detached || main.Locked || main.Prunable {
		t.Errorf("entries[0] = %+v, want no flags set", main)
	}

	feature := entries[1]
	if feature.Branch != "refs/heads/feature/my-feature" {
		t.Errorf("entries[1].Branch = %q, want refs/heads/feature/my-feature", feature.Branch)
	}

	detached := entries[2]
	if !detached.Detached {
		t.Errorf("entries[2].Detached = false, want true")
	}
	if detached.Branch != "" {
		t.Errorf("entries[2].Branch = %q, want empty for a detached worktree", detached.Branch)
	}

	locked := entries[3]
	if !locked.Locked {
		t.Errorf("entries[3].Locked = false, want true")
	}

	prunable := entries[4]
	if !prunable.Prunable {
		t.Errorf("entries[4].Prunable = false, want true")
	}
}

func TestParseWorktreePorcelain_SingleEntryNoTrailingBlankLine(t *testing.T) {
	// git's actual output has no trailing blank line after the last entry;
	// make sure the final flush still captures it.
	output := []byte(`worktree /home/user/projects/myapp
HEAD abcdef0123456789abcdef0123456789abcdef01
branch refs/heads/main`)

	entries := parseWorktreePorcelain(output)
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Path != "/home/user/projects/myapp" {
		t.Errorf("entries[0].Path = %q, want main repository path", entries[0].Path)
	}
}

func TestParseWorktreePorcelain_Empty(t *testing.T) {
	entries := parseWorktreePorcelain([]byte(""))
	if len(entries) != 0 {
		t.Errorf("parseWorktreePorcelain(\"\") = %v, want empty", entries)
	}
}
