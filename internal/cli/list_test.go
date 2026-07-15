package cli

import (
	"path/filepath"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

// TestListAllRepos_FlagsStaleClaims exercises listAllRepos directly against
// a registry containing one live claim and one whose worktree has been
// removed out from under it, guarding DESIGN.md's "list must flag it as
// stale" rule (rather than silently reporting a stale claim with blank
// fields, or dropping it).
func TestListAllRepos_FlagsStaleClaims(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	store, err := registry.Open()
	if err != nil {
		t.Fatalf("registry.Open() returned unexpected error: %v", err)
	}

	liveWorktree := t.TempDir()
	liveInstance := instance.New(instance.NewParams{
		Project: "myapp", Name: "live", Branch: "feature/live",
		MainRepository: "/repo", Path: liveWorktree, BlockStart: 4000,
		Ports: map[string]int{"HTTP_PORT": 4000},
	})
	if err := liveInstance.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}
	if _, err := store.Claim(registry.ClaimRequest{
		Project: "myapp", Instance: "live", Repository: "/repo", Worktree: liveWorktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("Claim() (live) returned unexpected error: %v", err)
	}

	staleWorktree := filepath.Join(t.TempDir(), "removed-worktree")
	if _, err := store.Claim(registry.ClaimRequest{
		Project: "myapp", Instance: "stale", Repository: "/repo", Worktree: staleWorktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("Claim() (stale) returned unexpected error: %v", err)
	}

	entries, err := listAllRepos()
	if err != nil {
		t.Fatalf("listAllRepos() returned unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	var live, stale *listEntry
	for i := range entries {
		switch entries[i].Instance {
		case "live":
			live = &entries[i]
		case "stale":
			stale = &entries[i]
		}
	}

	if live == nil {
		t.Fatalf("no entry for the live instance: %+v", entries)
	}
	if live.Stale {
		t.Errorf("live entry Stale = true, want false")
	}
	if live.Branch != "feature/live" {
		t.Errorf("live entry Branch = %q, want feature/live", live.Branch)
	}

	if stale == nil {
		t.Fatalf("no entry for the stale instance: %+v", entries)
	}
	if !stale.Stale {
		t.Errorf("stale entry Stale = false, want true")
	}
	if stale.Path != staleWorktree {
		t.Errorf("stale entry Path = %q, want %q", stale.Path, staleWorktree)
	}
}
