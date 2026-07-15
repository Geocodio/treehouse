package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/instance"
)

func newPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path <name>",
		Short: "Print an instance's worktree directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPath(cmd, args[0])
		},
	}
}

func runPath(cmd *cobra.Command, name string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	repo, err := loadRepositoryContext(cwd)
	if err != nil {
		return err
	}

	instances, err := instance.Discover(repo.MainRepository)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	for _, candidate := range instances {
		if candidate.Name == name {
			fmt.Fprintln(cmd.OutOrStdout(), candidate.Path)
			return nil
		}
	}

	return NewExitError(ExitGenericFailure, fmt.Errorf("no instance named %q", name))
}
