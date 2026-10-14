package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
)

func newDownCommand() *cobra.Command {
	var removeVolumes, keepVolumes bool

	command := &cobra.Command{
		Use:   "down",
		Short: "Bring the compose stack down",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDown(cmd, removeVolumes)
		},
	}

	command.Flags().BoolVar(&removeVolumes, "volumes", false, "also remove the compose stack's named volumes (deletes its data)")
	command.Flags().BoolVar(&keepVolumes, "keep-volumes", false, "")
	_ = command.Flags().MarkDeprecated("keep-volumes", "down keeps volumes by default; pass --volumes to remove them")

	return command
}

func runDown(cmd *cobra.Command, removeVolumes bool) error {
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

	if err := runner.Down(inst, removeVolumes, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}
