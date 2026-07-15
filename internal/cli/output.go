package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/manifest"
)

// printWarnings prints manifest warnings (unknown nested keys, etc.) to
// stderr, one per line, prefixed so they're visibly distinct from other
// output.
func printWarnings(cmd *cobra.Command, warnings []manifest.Warning) {
	for _, warning := range warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warning.String())
	}
}

// printJSON writes v to out as a single indented JSON document, per
// DESIGN.md's "--json on create/list emits a single JSON document on
// stdout" rule.
func printJSON(cmd *cobra.Command, v any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
