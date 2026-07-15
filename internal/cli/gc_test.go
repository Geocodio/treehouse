package cli

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/geocodio/treehouse/internal/registry"
)

func TestRunGCPass_ReapsStaleClaimsEvenWithoutDocker(t *testing.T) {
	store := registry.NewStore(t.TempDir())

	staleWorktree := filepath.Join(t.TempDir(), "removed-worktree")
	if _, err := store.Claim(registry.ClaimRequest{
		Project: "myapp", Instance: "stale", Repository: "/repo", Worktree: staleWorktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var warnings bytes.Buffer
	report, err := runGCPass(store, 24*time.Hour, false, &warnings)
	if err != nil {
		t.Fatalf("runGCPass() returned unexpected error: %v", err)
	}

	if len(report.Reaped) != 1 {
		t.Fatalf("report.Reaped = %v, want exactly the stale claim", report.Reaped)
	}

	remaining, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("List() after runGCPass() = %+v, want empty (stale claim reaped)", remaining)
	}
}

func TestRunGCPass_DryRunReapsNothing(t *testing.T) {
	store := registry.NewStore(t.TempDir())

	staleWorktree := filepath.Join(t.TempDir(), "removed-worktree")
	if _, err := store.Claim(registry.ClaimRequest{
		Project: "myapp", Instance: "stale", Repository: "/repo", Worktree: staleWorktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var warnings bytes.Buffer
	report, err := runGCPass(store, 24*time.Hour, true, &warnings)
	if err != nil {
		t.Fatalf("runGCPass() returned unexpected error: %v", err)
	}

	if len(report.Reaped) != 1 {
		t.Fatalf("report.Reaped = %v, want the stale claim reported even in dry-run", report.Reaped)
	}

	remaining, err := store.List()
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("List() after a dry-run runGCPass() = %+v, want the claim still present", remaining)
	}
}

func TestRunGCPass_LiveClaimBelowThresholdIsUntouched(t *testing.T) {
	store := registry.NewStore(t.TempDir())

	liveWorktree := initIdleTestRepository(t)
	if _, err := store.Claim(registry.ClaimRequest{
		Project: "myapp", Instance: "live", Repository: "/repo", Worktree: liveWorktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var warnings bytes.Buffer
	report, err := runGCPass(store, 24*time.Hour, false, &warnings)
	if err != nil {
		t.Fatalf("runGCPass() returned unexpected error: %v", err)
	}

	if len(report.Stopped) != 0 {
		t.Errorf("report.Stopped = %v, want empty for a freshly active worktree", report.Stopped)
	}
	if len(report.Reaped) != 0 {
		t.Errorf("report.Reaped = %v, want empty for a worktree that still exists", report.Reaped)
	}
}
