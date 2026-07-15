package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/provision"
)

func newProvisionCommand() *cobra.Command {
	var retry bool

	command := &cobra.Command{
		Use:   "provision",
		Short: "Re-run provisioning for the current instance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProvision(cmd, retry)
		},
	}

	command.Flags().BoolVar(&retry, "retry", false, "re-run provisioning from the instance's recorded state")

	return command
}

// runProvision resumes provisioning from inst.State: MaterializeDependencies
// only runs when still at StateWorktreeCreated (it's idempotent, but
// there's no need to repeat it once past that state), while Reconcile and
// RunHook always run — that is what makes --retry meaningful after a
// provision-failed hook, and what lets Reconcile pick up a lockfile that
// drifted after the instance was already provisioned.
func runProvision(cmd *cobra.Command, retry bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	inst, err := compose.ResolveInstance(cwd)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}
	if inst == nil {
		return NewExitError(ExitNotTreehouseRepository, fmt.Errorf("provision must be run from inside an instance"))
	}

	manifestConfig, err := loadManifestFor(inst.MainRepository)
	if err != nil {
		return err
	}

	if !retry && inst.State != instance.StateWorktreeCreated {
		fmt.Fprintf(cmd.ErrOrStderr(), "instance %q is already at state %q; rerun with --retry to re-provision\n", inst.Name, inst.State)
		return nil
	}

	if inst.State == instance.StateWorktreeCreated {
		if err := provision.MaterializeDependencies(inst, manifestConfig); err != nil {
			return NewExitError(ExitGenericFailure, err)
		}
		inst.AdvanceState(instance.StateDepsCloned)
		if err := inst.Save(); err != nil {
			return NewExitError(ExitGenericFailure, err)
		}
	}

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

	fmt.Fprintf(cmd.OutOrStdout(), "instance %q provisioned\n", inst.Name)
	return nil
}
