package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the compose stack's status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd)
		},
	}
}

func runStatus(cmd *cobra.Command) error {
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
		if err := runner.Compose(nil, []string{"ps"}, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return NewExitError(ExitGenericFailure, err)
		}
		return nil
	}

	count, err := compose.RunningContainerCount(inst.ComposeProject)
	if err != nil {
		return NewExitError(ExitDockerUnavailable, err)
	}

	fmt.Fprintf(
		cmd.OutOrStdout(),
		"instance %s (branch %s)\nstate: %s\nrunning containers: %d\n",
		inst.Name, inst.Branch, inst.State, count,
	)

	return nil
}
