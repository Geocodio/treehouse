package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCommand prints the version string baked into the binary via
// -ldflags at build time (see the goreleaser and Makefile build targets).
func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the treehouse version",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			fmt.Fprintln(command.OutOrStdout(), version)
			return nil
		},
	}
}
