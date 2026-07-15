package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
)

func newComposeCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "compose -- <args...>",
		Short:              "Run docker compose, with --env-file/--project-name injected inside an instance",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runComposePassthrough(cmd, args)
		},
	}
}

func runComposePassthrough(cmd *cobra.Command, args []string) error {
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

	if err := runner.Compose(inst, args, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}
