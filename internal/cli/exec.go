package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
)

func newExecCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "exec <service> -- <command...>",
		Short:              "Run a command inside a service's running container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExec(cmd, args)
		},
	}
}

// runExec is sugar for `treehouse compose exec <service> <command...>`.
func runExec(cmd *cobra.Command, args []string) error {
	service, command, err := parseExecArgs(args)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

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

	if err := runner.Exec(inst, service, command, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}

// parseExecArgs splits "<service> [--] <command...>" into a service name
// and the command to run, accepting (but not requiring) an explicit "--"
// separator right after the service name.
func parseExecArgs(args []string) (service string, command []string, err error) {
	if len(args) < 1 {
		return "", nil, fmt.Errorf("usage: treehouse exec <service> [--] <command...>")
	}

	service = args[0]
	command = args[1:]
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}

	if len(command) == 0 {
		return "", nil, fmt.Errorf("exec requires a command to run")
	}

	return service, command, nil
}
