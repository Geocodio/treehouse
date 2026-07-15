package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
)

func newRestartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the compose stack",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestart(cmd)
		},
	}
}

func runRestart(cmd *cobra.Command) error {
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

	if err := runner.Restart(inst, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}
