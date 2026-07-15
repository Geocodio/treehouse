package cli

import "github.com/spf13/cobra"

// NewRootCommand builds the treehouse root cobra command, wiring version
// information, exit-code mapping (see ExitCodeFor), and the instance
// lifecycle commands documented in DESIGN.md's "Command behavior" section.
// gc, prune, init, doctor, and switch are wave 3 and not registered here.
func NewRootCommand(version string) *cobra.Command {
	rootCommand := &cobra.Command{
		Use:   "treehouse",
		Short: "Isolated docker-compose stacks per git worktree",
		// Errors and usage are printed by the caller in cmd/treehouse/main.go
		// so that failures can be mapped to a contractual exit code first.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCommand.AddCommand(newVersionCommand(version))
	rootCommand.AddCommand(newCreateCommand())
	rootCommand.AddCommand(newListCommand())
	rootCommand.AddCommand(newRemoveCommand())
	rootCommand.AddCommand(newPathCommand())
	rootCommand.AddCommand(newUpCommand())
	rootCommand.AddCommand(newDownCommand())
	rootCommand.AddCommand(newRestartCommand())
	rootCommand.AddCommand(newStatusCommand())
	rootCommand.AddCommand(newComposeCommand())
	rootCommand.AddCommand(newExecCommand())
	rootCommand.AddCommand(newProvisionCommand())

	return rootCommand
}
