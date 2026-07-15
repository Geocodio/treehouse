package cli

import "github.com/spf13/cobra"

// NewRootCommand builds the treehouse root cobra command, wiring version
// information and exit-code mapping (see ExitCodeFor). Wave 1 registers only
// the version subcommand; later waves add create, list, up, down, and the
// rest of the commands documented in DESIGN.md's "Command behavior" section.
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

	return rootCommand
}
