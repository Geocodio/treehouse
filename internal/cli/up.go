package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
)

func newUpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Bring the compose stack up",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(cmd)
		},
	}
}

// runUp is context-aware per DESIGN.md: inside an instance, it re
// -verifies the instance's port block before starting (compose.Up) and
// advances TREEHOUSE_STATE; in a main repository, it's a thin passthrough
// to `docker compose up -d`.
func runUp(cmd *cobra.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	inst, err := compose.ResolveInstance(cwd)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	runner, err := compose.NewRunner()
	if err != nil {
		return NewExitError(ExitDockerUnavailable, err)
	}

	if inst == nil {
		if err := runner.Compose(nil, []string{"up", "-d"}, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return NewExitError(ExitGenericFailure, err)
		}
		return nil
	}

	manifestConfig, err := loadManifestFor(inst.MainRepository)
	if err != nil {
		return err
	}

	if err := runner.Up(inst, manifestConfig.Ports.BlockSize, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	inst.AdvanceState(instance.StateStackUp)
	if err := inst.Save(); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}
