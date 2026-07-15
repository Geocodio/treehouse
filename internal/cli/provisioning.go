package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/manifest"
	"github.com/geocodio/treehouse/internal/provision"
)

// This file holds the provisioning pipeline stages shared by `create` and
// `treehouse provision`, in the fixed order DESIGN.md's state machine
// requires: worktree-created -> deps-cloned -> stack-up -> provisioned. The
// hook only ever runs once the compose stack is up — real-world hooks run
// database migrations that need database containers running — so
// materializeDeps, ensureStackUp, and reconcileAndHook are three separate
// steps rather than one, letting each caller stop wherever its own flags
// (create's --no-start/--no-provision) or state (provision's "resume from
// here") say to.

// materializeDeps runs provision.MaterializeDependencies and advances inst
// to StateDepsCloned, saving the env file so the transition and the
// dependency clone land together.
func materializeDeps(cmd *cobra.Command, inst *instance.Instance, manifestConfig *manifest.Manifest) error {
	if err := provision.MaterializeDependencies(inst, manifestConfig, cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	inst.AdvanceState(instance.StateDepsCloned)
	if err := inst.Save(); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}

// ensureStackUp brings inst's compose stack up (compose.Runner.Up is
// idempotent: it re-verifies the port block and issues `up -d` whether or
// not the stack is already running) and advances inst to StateStackUp. Both
// `create` and `treehouse provision` need this immediately before running
// the post-create hook, per DESIGN.md's "provision must bring the stack up
// first if it isn't" rule.
//
// Unlike `treehouse up`'s own call to runner.Up (internal/cli/up.go), whose
// entire purpose is relaying the compose stack's output, this call site is
// reused inside `create`'s pipeline — including `create --json`, which
// writes a single JSON document to this same cmd.OutOrStdout() once
// provisioning finishes. So the stack's own chatter is routed to stderr
// here, never stdout, to keep that document uncorrupted.
func ensureStackUp(cmd *cobra.Command, inst *instance.Instance, manifestConfig *manifest.Manifest) error {
	runner, err := compose.NewRunner()
	if err != nil {
		return NewExitError(ExitDockerUnavailable, err)
	}

	if err := runner.Up(inst, manifestConfig.Ports.BlockSize, cmd.ErrOrStderr(), cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	inst.AdvanceState(instance.StateStackUp)
	if err := inst.Save(); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}

// reconcileAndHook runs provision.Reconcile then provision.RunHook — the
// pipeline's final stage, only ever reached once the stack is up — and
// advances inst to StateProvisioned. A failure in either step records
// StateProvisionFailed instead (via failProvisioning) so the instance is
// left inspectable, per DESIGN.md's "no automatic rollback" rule.
func reconcileAndHook(inst *instance.Instance, manifestConfig *manifest.Manifest) error {
	if err := provision.Reconcile(inst, manifestConfig); err != nil {
		return failProvisioning(inst, err)
	}

	if err := provision.RunHook(inst, manifestConfig); err != nil {
		return failProvisioning(inst, err)
	}

	inst.AdvanceState(instance.StateProvisioned)
	if err := inst.Save(); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}

func failProvisioning(inst *instance.Instance, cause error) error {
	inst.AdvanceState(instance.StateProvisionFailed)
	if saveErr := inst.Save(); saveErr != nil {
		return NewExitError(ExitGenericFailure, fmt.Errorf("%w (also failed to record provision-failed state: %v)", cause, saveErr))
	}

	return NewExitError(ExitProvisionFailed, cause)
}
