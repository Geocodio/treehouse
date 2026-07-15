// Command treehouse gives every git branch its own worktree with an
// isolated docker-compose stack. See docs/DESIGN.md for the full contract.
package main

import (
	"fmt"
	"os"

	"github.com/geocodio/treehouse/internal/cli"
)

// version is overwritten at build time via -ldflags "-X main.version=...";
// see the Makefile and .goreleaser.yml build configuration.
var version = "dev"

func main() {
	rootCommand := cli.NewRootCommand(version)

	if err := rootCommand.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(int(cli.ExitCodeFor(err)))
	}
}
