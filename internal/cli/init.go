package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/instance"
)

// composeFileCandidates are the file names runInit looks for, in order.
var composeFileCandidates = []string{"docker-compose.yml", "docker-compose.yaml"}

func newInitCommand() *cobra.Command {
	var write bool

	command := &cobra.Command{
		Use:   "init",
		Short: "Propose a .treehouse.yml from the current directory's docker-compose.yml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, write)
		},
	}

	command.Flags().BoolVar(&write, "write", false, "save the proposed manifest to .treehouse.yml (refuses to overwrite an existing one)")

	return command
}

// runInit never edits docker-compose.yml — it only reads it, proposes a
// .treehouse.yml, and (for a hardcoded published port) prints the exact
// diff needed to parameterize it, per DESIGN.md's "init ... never edits the
// compose file" rule.
func runInit(cmd *cobra.Command, write bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	composePath, err := findComposeFile(cwd)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	services, err := parseComposeServices(data)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	var ports []publishedPort
	for _, service := range services {
		for _, raw := range service.Ports {
			if parsed := parsePortEntry(service.Name, raw); parsed != nil {
				ports = append(ports, *parsed)
			}
		}
	}

	projectName := instance.Slugify(filepath.Base(cwd))
	envNames, diffs := proposeManifestPorts(ports)

	printInitReport(cmd, projectName, envNames, diffs)

	if write {
		return writeProposedManifest(cwd, projectName, envNames)
	}

	return nil
}

func findComposeFile(dir string) (string, error) {
	for _, candidate := range composeFileCandidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no docker-compose.yml (or .yaml) found in %s", dir)
}

// proposeManifestPorts walks ports in discovery order, assigning each a
// ports.env name (adopting an existing "${VAR:-default}" substitution's VAR
// as-is, or proposing a new name derived from the service for a hardcoded
// host port) and a sequential index — envNames[i] belongs at index i in the
// proposed manifest's ports.env. diffs holds one compose-diff line per
// hardcoded port that needs parameterizing.
func proposeManifestPorts(ports []publishedPort) (envNames []string, diffs []string) {
	used := map[string]bool{}

	for _, port := range ports {
		switch {
		case port.ExistingVar != "":
			used[port.ExistingVar] = true
			envNames = append(envNames, port.ExistingVar)

		case port.HardcodedHost != "":
			name := deriveVarName(port.ServiceName, used)
			envNames = append(envNames, name)
			diffs = append(diffs, buildPortDiff(port, name))
		}
	}

	return envNames, diffs
}

func buildPortDiff(port publishedPort, varName string) string {
	proposed := strings.Replace(port.Raw, port.HardcodedHost+":", fmt.Sprintf("${%s:-%s}:", varName, port.HardcodedHost), 1)

	return fmt.Sprintf(
		"services.%s.ports:\n  - %q\n  + %q",
		port.ServiceName, port.Raw, proposed,
	)
}

var nonEnvNameCharacters = regexp.MustCompile(`[^A-Z0-9]`)

// deriveVarName proposes a ports.env variable name for a hardcoded port on
// serviceName, e.g. "web" -> WEB_PORT, "cache" -> CACHE_PORT, appending a
// numeric suffix (WEB_PORT_2, ...) if the service publishes more than one
// hardcoded port or the base name is already taken (by another service, or
// by a var name already adopted from an existing "${VAR:-default}").
func deriveVarName(serviceName string, used map[string]bool) string {
	base := nonEnvNameCharacters.ReplaceAllString(strings.ToUpper(serviceName), "_") + "_PORT"

	if !used[base] {
		used[base] = true
		return base
	}

	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", base, suffix)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// renderManifest hand-builds the proposed .treehouse.yml's exact text
// (rather than yaml.Marshal-ing a struct) so ports.env keeps envNames'
// deterministic, readable discovery order instead of Go map iteration
// order.
func renderManifest(project string, envNames []string) string {
	var builder strings.Builder

	builder.WriteString("version: 1\n")
	fmt.Fprintf(&builder, "project: %s\n", project)

	if len(envNames) > 0 {
		builder.WriteString("\nports:\n")
		builder.WriteString("  block_size: 32\n")
		builder.WriteString("  env:\n")
		for index, name := range envNames {
			fmt.Fprintf(&builder, "    %s: %d\n", name, index)
		}
	}

	return builder.String()
}

func writeProposedManifest(dir, project string, envNames []string) error {
	path := filepath.Join(dir, manifestFileName)
	if _, err := os.Stat(path); err == nil {
		return NewExitError(ExitGenericFailure, fmt.Errorf("%s already exists; refusing to overwrite", path))
	}

	if err := os.WriteFile(path, []byte(renderManifest(project, envNames)), 0o644); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	return nil
}

func printInitReport(cmd *cobra.Command, project string, envNames []string, diffs []string) {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "proposed .treehouse.yml:")
	fmt.Fprintln(out, "---")
	fmt.Fprint(out, renderManifest(project, envNames))
	fmt.Fprintln(out, "---")

	if len(diffs) == 0 {
		return
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "docker-compose.yml changes needed (init never edits the file itself):")
	for _, diff := range diffs {
		fmt.Fprintln(out, diff)
	}
}
