package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
)

func newDownCommand() *cobra.Command {
	var keepVolumes bool

	command := &cobra.Command{
		Use:   "down",
		Short: "Bring the compose stack down",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDown(cmd, keepVolumes)
		},
	}

	command.Flags().BoolVar(&keepVolumes, "keep-volumes", false, "don't remove the compose stack's named volumes")

	return command
}

func runDown(cmd *cobra.Command, keepVolumes bool) error {
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

	if err := runner.Down(inst, !keepVolumes, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}
