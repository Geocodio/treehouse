package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

// DefaultIdleHours is gc's default idle threshold, per DESIGN.md.
const DefaultIdleHours = 24

func newGCCommand() *cobra.Command {
	var idleHours int
	var dryRun bool

	command := &cobra.Command{
		Use:   "gc",
		Short: "Stop idle instances and reap stale registry claims",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGC(cmd, idleHours, dryRun)
		},
	}

	command.Flags().IntVar(&idleHours, "idle-hours", DefaultIdleHours, "stop instances idle longer than this many hours")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "report what would happen without stopping or reaping anything")

	return command
}

func runGC(cmd *cobra.Command, idleHours int, dryRun bool) error {
	store, err := registry.Open()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	report, err := runGCPass(store, time.Duration(idleHours)*time.Hour, dryRun, cmd.ErrOrStderr())
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	printGCReport(cmd, report, dryRun)
	return nil
}

// gcReport is what a gc pass stopped/reaped (or, in --dry-run, would have),
// for both the standalone `gc` command's report and opportunistic gc's
// terse stderr summary.
type gcReport struct {
	Stopped []string
	Reaped  []string
}

// runGCPass is gc's core logic, independent of any specific cobra command:
// `treehouse gc` and the opportunistic pass create/list trigger both call
// this, differing only in idleThreshold/dryRun and how they present the
// result. Per-instance failures (docker unreachable for one project, etc.)
// are written to warnings and skipped rather than aborting the whole pass;
// only a registry-level failure (open/list/GC itself) is returned as an
// error.
func runGCPass(store *registry.Store, idleThreshold time.Duration, dryRun bool, warnings io.Writer) (gcReport, error) {
	var report gcReport

	claims, err := store.List()
	if err != nil {
		return report, err
	}

	now := time.Now()

	for _, claim := range claims {
		if _, statErr := os.Stat(claim.Worktree); statErr != nil {
			report.Reaped = append(report.Reaped, fmt.Sprintf("%s/%s (%s)", claim.Project, claim.Instance, claim.Worktree))
			continue
		}

		idle, reason, err := isIdleBeyond(claim, idleThreshold, now)
		if err != nil {
			fmt.Fprintf(warnings, "warning: checking idle state for %s/%s: %v\n", claim.Project, claim.Instance, err)
			continue
		}
		if !idle {
			continue
		}

		composeProject := claim.Project + "-" + claim.Instance
		count, countErr := compose.RunningContainerCount(composeProject)
		if countErr == nil && count == 0 {
			continue // already stopped, nothing to do
		}

		report.Stopped = append(report.Stopped, fmt.Sprintf("%s/%s (idle %s)", claim.Project, claim.Instance, reason))

		if dryRun {
			continue
		}

		if err := stopComposeProjectForClaim(claim, warnings); err != nil {
			fmt.Fprintf(warnings, "warning: stopping %s/%s: %v\n", claim.Project, claim.Instance, err)
		}
	}

	if !dryRun {
		// Reuses the registry's own stale-claim GC (which also updates
		// last_gc_at, rate-limiting future opportunistic passes) rather
		// than duplicating that logic here.
		if _, err := store.GC(); err != nil {
			return report, err
		}
	}

	return report, nil
}

func stopComposeProjectForClaim(claim registry.Claim, stderr io.Writer) error {
	envFilePath := filepath.Join(claim.Worktree, instance.EnvFileName)
	inst, err := instance.LoadEnvFile(envFilePath)
	if err != nil {
		return err
	}

	runner, err := compose.NewRunner()
	if err != nil {
		return err
	}

	return runner.Stop(inst, io.Discard, stderr)
}

func printGCReport(cmd *cobra.Command, report gcReport, dryRun bool) {
	out := cmd.OutOrStdout()
	verb := "stopped"
	if dryRun {
		verb = "would stop"
	}

	if len(report.Stopped) == 0 {
		fmt.Fprintf(out, "gc: no idle instances to stop\n")
	} else {
		fmt.Fprintf(out, "gc: %s %d idle instance(s):\n", verb, len(report.Stopped))
		for _, line := range report.Stopped {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}

	reapVerb := "reaped"
	if dryRun {
		reapVerb = "would reap"
	}

	if len(report.Reaped) == 0 {
		fmt.Fprintf(out, "gc: no stale claims to reap\n")
	} else {
		fmt.Fprintf(out, "gc: %s %d stale claim(s):\n", reapVerb, len(report.Reaped))
		for _, line := range report.Reaped {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
}

// maybeRunOpportunisticGC runs gc's pass silently (its per-item detail goes
// to stderr as warnings only on failure; a one-line summary is printed to
// stderr when it actually did something), per DESIGN.md: "Every create/list
// invocation opportunistically runs gc ... unless --no-gc or gc ran within
// the last hour", and "Failures in opportunistic gc are warnings, never
// fatal." noGC is create/list's own --no-gc flag.
func maybeRunOpportunisticGC(cmd *cobra.Command, noGC bool) {
	if noGC {
		return
	}

	store, err := registry.Open()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: opportunistic gc:", err)
		return
	}

	lastGCAt, err := store.LastGCAt()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: opportunistic gc:", err)
		return
	}
	if !registry.NeedsGC(lastGCAt, time.Now()) {
		return
	}

	report, err := runGCPass(store, DefaultIdleHours*time.Hour, false, cmd.ErrOrStderr())
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: opportunistic gc:", err)
		return
	}

	if len(report.Stopped) > 0 || len(report.Reaped) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "gc: stopped %d idle instance(s), reaped %d stale claim(s)\n", len(report.Stopped), len(report.Reaped))
	}
}
