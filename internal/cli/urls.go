package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
)

func newURLsCommand() *cobra.Command {
	var jsonOutput bool

	command := &cobra.Command{
		Use:   "urls [name]",
		Short: "Print an instance's local URLs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runURLs(cmd, name, jsonOutput)
		},
	}

	command.Flags().BoolVar(&jsonOutput, "json", false, "emit a single JSON document on stdout")

	return command
}

// urlsResult is urls' --json output shape. Its "urls" field reuses
// instance.RenderedURL so it matches create --json's "urls" field exactly.
type urlsResult struct {
	Instance string                 `json:"instance"`
	URLs     []instance.RenderedURL `json:"urls"`
}

func runURLs(cmd *cobra.Command, name string, jsonOutput bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	target, err := resolveURLsTarget(cwd, name)
	if err != nil {
		return err
	}

	// The manifest is read from the instance's own worktree, the same way
	// up/provision resolve it (see loadManifestFor).
	manifestConfig, err := loadManifestFor(target.Path)
	if err != nil {
		return err
	}

	result := urlsResult{
		Instance: target.Name,
		URLs:     instance.RenderURLs(manifestConfig.URLs, target.Ports),
	}

	if jsonOutput {
		return printJSON(cmd, result)
	}

	printURLs(cmd, result)
	return nil
}

// resolveURLsTarget finds the instance whose URLs to print: by name if
// given, or by cwd if it's inside one.
func resolveURLsTarget(cwd, name string) (*instance.Instance, error) {
	if name == "" {
		target, err := compose.ResolveInstance(cwd)
		if err != nil {
			return nil, NewExitError(ExitGenericFailure, err)
		}
		if target == nil {
			return nil, NewExitError(ExitGenericFailure, fmt.Errorf("no instance name given and cwd is not inside one"))
		}
		return target, nil
	}

	repo, err := loadRepositoryContext(cwd)
	if err != nil {
		return nil, err
	}

	instances, err := instance.Discover(repo.MainRepository)
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	for i := range instances {
		if instances[i].Name == name {
			return &instances[i], nil
		}
	}

	return nil, NewExitError(ExitGenericFailure, fmt.Errorf("no instance named %q", name))
}

// printURLs writes one aligned "name  url" line per URL to stdout. A
// manifest with no urls section prints a note to stderr instead, so stdout
// stays empty for scripts.
func printURLs(cmd *cobra.Command, result urlsResult) {
	if len(result.URLs) == 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "instance %q has no urls configured in %s\n", result.Instance, manifestFileName)
		return
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, url := range result.URLs {
		fmt.Fprintf(writer, "%s\t%s\n", url.Name, url.URL)
	}
	writer.Flush()
}
