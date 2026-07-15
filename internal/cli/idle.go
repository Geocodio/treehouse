package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/gitx"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

// instanceActivity resolves the newest of claim's three idle-detection
// signals, per DESIGN.md's "gc idle detection" rule: container activity
// (StartedAt/FinishedAt across every container in the project), the
// worktree's git HEAD reflog mtime, and the .treehouse.env mtime. It
// returns an error only when every signal is unavailable — an individual
// signal failing (e.g. docker being unreachable) is tolerated, since the
// other two are still meaningful. source names whichever signal won, for
// gc/prune's reporting.
//
// Shared by `gc` (idle-stop) and `prune` (idle-remove), which differ only in
// which threshold they compare the result against and what action they take
// once something is idle past it.
func instanceActivity(claim registry.Claim) (newest time.Time, source string, err error) {
	composeProject := claim.Project + "-" + claim.Instance

	if activity, activityErr := compose.ContainerActivityTimestamp(composeProject); activityErr == nil && activity.After(newest) {
		newest = activity
		source = "containers"
	}

	if reflogTime, ok, reflogErr := gitx.ReflogHeadMTime(claim.Worktree); reflogErr == nil && ok && reflogTime.After(newest) {
		newest = reflogTime
		source = "git HEAD"
	}

	envPath := filepath.Join(claim.Worktree, instance.EnvFileName)
	if info, statErr := os.Stat(envPath); statErr == nil && info.ModTime().After(newest) {
		newest = info.ModTime()
		source = ".treehouse.env"
	}

	if newest.IsZero() {
		return time.Time{}, "", fmt.Errorf("no activity signal available for %s/%s", claim.Project, claim.Instance)
	}

	return newest, source, nil
}

// isIdleBeyond reports whether claim's newest activity signal is older than
// threshold as of now, alongside a human-readable reason for gc/prune's
// reporting.
func isIdleBeyond(claim registry.Claim, threshold time.Duration, now time.Time) (idle bool, reason string, err error) {
	newest, source, err := instanceActivity(claim)
	if err != nil {
		return false, "", err
	}

	age := now.Sub(newest)

	return age >= threshold, fmt.Sprintf("%s since last %s activity", age.Round(time.Minute), source), nil
}
