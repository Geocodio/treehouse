package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/gitx"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

// DefaultPruneDays is prune's default idle threshold, per DESIGN.md.
const DefaultPruneDays = 30

func newPruneCommand() *cobra.Command {
	var days int
	var dryRun, force bool

	command := &cobra.Command{
		Use:   "prune",
		Short: "Remove instances untouched for --days days",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrune(cmd, days, dryRun, force)
		},
	}

	command.Flags().IntVar(&days, "days", DefaultPruneDays, "remove instances idle longer than this many days")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be removed without removing anything")
	command.Flags().BoolVar(&force, "force", false, "skip the per-instance confirmation prompt (a dirty worktree is still never removed)")

	return command
}

// runPrune removes every registry claim idle longer than --days, via the
// exact same code path as `remove` (stopInstanceStack, removeWorktree,
// releaseClaim — see prunOneInstance), so a dirty worktree is always
// treated as in-use and skipped rather than force-removed: unlike `remove
// --force`, prune's --force only ever skips the confirmation prompt, never
// the dirty-tree refusal. Per DESIGN.md, every candidate gets its own
// prompt — prune never bulk-confirms multiple removals with one y/N — and
// the first candidate that would need a prompt without a TTY (and no
// --force) aborts the whole command with ExitWouldPrompt rather than
// silently skipping it or looping forever.
func runPrune(cmd *cobra.Command, days int, dryRun, force bool) error {
	store, err := registry.Open()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	claims, err := store.List()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	threshold := time.Duration(days) * 24 * time.Hour
	now := time.Now()

	var removed, skipped []string

	for _, claim := range claims {
		if _, statErr := os.Stat(claim.Worktree); statErr != nil {
			// A registry claim with no worktree left isn't something prune
			// removes (there's no worktree to remove) — that's gc's stale
			// -claim reaping, so leave it alone here.
			continue
		}

		idle, reason, err := isIdleBeyond(claim, threshold, now)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: checking %s/%s: %v\n", claim.Project, claim.Instance, err)
			continue
		}
		if !idle {
			continue
		}

		dirty, err := gitx.IsDirty(claim.Worktree)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: checking worktree status for %s/%s: %v\n", claim.Project, claim.Instance, err)
			continue
		}
		if dirty {
			skipped = append(skipped, fmt.Sprintf("%s/%s: dirty worktree (in use)", claim.Project, claim.Instance))
			continue
		}

		if dryRun {
			removed = append(removed, fmt.Sprintf("%s/%s (idle %s)", claim.Project, claim.Instance, reason))
			continue
		}

		if !force {
			if !IsTTY() {
				return NewExitError(
					ExitWouldPrompt,
					fmt.Errorf("instance %s/%s needs confirmation to prune; rerun with --force in non-interactive mode", claim.Project, claim.Instance),
				)
			}
			if !confirm(fmt.Sprintf("prune instance %s/%s at %s (idle %s)?", claim.Project, claim.Instance, claim.Worktree, reason)) {
				skipped = append(skipped, fmt.Sprintf("%s/%s: declined", claim.Project, claim.Instance))
				continue
			}
		}

		if err := prunOneInstance(cmd, claim); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: pruning %s/%s: %v\n", claim.Project, claim.Instance, err)
			continue
		}

		removed = append(removed, fmt.Sprintf("%s/%s (idle %s)", claim.Project, claim.Instance, reason))
	}

	printPruneReport(cmd, removed, skipped, dryRun)
	return nil
}

// prunOneInstance removes a single idle, clean instance via the exact same
// steps `remove` itself uses: stop the stack, `git worktree remove` (still
// without --force — removeWorktree only forces when explicitly asked, and
// prune never asks), then release the port claim.
func prunOneInstance(cmd *cobra.Command, claim registry.Claim) error {
	envFilePath := filepath.Join(claim.Worktree, instance.EnvFileName)
	inst, err := instance.LoadEnvFile(envFilePath)
	if err != nil {
		return err
	}

	stopInstanceStack(cmd, inst, false)

	if err := removeWorktree(cmd, inst.MainRepository, inst, false); err != nil {
		return err
	}

	return releaseClaim(inst)
}

func printPruneReport(cmd *cobra.Command, removed, skipped []string, dryRun bool) {
	out := cmd.OutOrStdout()
	verb := "removed"
	if dryRun {
		verb = "would remove"
	}

	if len(removed) == 0 {
		fmt.Fprintln(out, "prune: no idle instances to remove")
	} else {
		fmt.Fprintf(out, "prune: %s %d instance(s):\n", verb, len(removed))
		for _, line := range removed {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}

	if len(skipped) > 0 {
		fmt.Fprintf(out, "prune: skipped %d instance(s):\n", len(skipped))
		for _, line := range skipped {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
}
