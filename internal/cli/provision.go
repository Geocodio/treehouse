package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
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

// runProvision resumes provisioning from inst.State, running the same
// materializeDeps -> ensureStackUp -> reconcileAndHook pipeline `create`
// does. Per DESIGN.md, `provision` must bring the stack up first if it
// isn't (completing an instance created with --no-start, which stops at
// deps-cloned) and then run the hook/reconcile (completing one created with
// --no-provision, which never ran them at all) — neither case requires
// --retry, since the instance was never fully provisioned to begin with.
// --retry is only required to re-run provisioning that already reached
// StateProvisioned or StateProvisionFailed.
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

	alreadyDone := inst.State == instance.StateProvisioned || inst.State == instance.StateProvisionFailed
	if !retry && alreadyDone {
		fmt.Fprintf(cmd.ErrOrStderr(), "instance %q is already at state %q; rerun with --retry to re-provision\n", inst.Name, inst.State)
		return nil
	}

	if inst.State == instance.StateWorktreeCreated {
		if err := materializeDeps(inst, manifestConfig); err != nil {
			return err
		}
	}

	if err := ensureStackUp(cmd, inst, manifestConfig); err != nil {
		return err
	}

	if err := reconcileAndHook(inst, manifestConfig); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "instance %q provisioned\n", inst.Name)
	return nil
}
