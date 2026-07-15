package manifest

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

var knownPortsKeys = map[string]bool{
	"block_size": true,
	"env":        true,
	"aliases":    true,
}

// parsePorts parses the manifest's ports section and enforces the
// validation rule that every ports.env index is unique and less than
// block_size, plus ports.aliases' rules (see validatePortAliases).
func parsePorts(node *yaml.Node, result *Manifest) ([]string, []Warning) {
	if node.Kind != yaml.MappingNode {
		return []string{"ports: must be a mapping"}, nil
	}

	var problems []string
	var warnings []Warning
	blockSize := DefaultBlockSize
	env := map[string]int{}
	aliases := map[string]string{}

	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]

		switch keyNode.Value {
		case "block_size":
			if err := valueNode.Decode(&blockSize); err != nil {
				problems = append(problems, fmt.Sprintf("ports.block_size: must be an integer (%s)", err))
				break
			}
			if blockSize <= 0 {
				problems = append(problems, fmt.Sprintf("ports.block_size: must be positive, got %d", blockSize))
			}
		case "env":
			envProblems := parsePortsEnv(valueNode, env)
			problems = append(problems, envProblems...)
		case "aliases":
			aliasProblems := parsePortsAliases(valueNode, aliases)
			problems = append(problems, aliasProblems...)
		default:
			if !knownPortsKeys[keyNode.Value] {
				warnings = append(warnings, Warning{
					Key:     "ports." + keyNode.Value,
					Message: fmt.Sprintf("unknown key (line %d); ignored", keyNode.Line),
				})
			}
		}
	}

	result.Ports.BlockSize = blockSize
	result.Ports.Env = env
	result.Ports.Aliases = aliases

	problems = append(problems, validatePortIndices(env, blockSize)...)
	problems = append(problems, validatePortAliases(env, aliases)...)

	return problems, warnings
}

// parsePortsAliases parses ports.aliases: a mapping of alias environment
// variable name to the canonical ports.env variable name it should mirror.
func parsePortsAliases(node *yaml.Node, aliases map[string]string) []string {
	if node.Kind != yaml.MappingNode {
		return []string{"ports.aliases: must be a mapping of alias environment variable name to canonical variable name"}
	}

	var problems []string

	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		name := keyNode.Value

		if err := validateEnvironmentVariableName(name); err != nil {
			problems = append(problems, err.Error())
			continue
		}

		var canonical string
		if err := valueNode.Decode(&canonical); err != nil {
			problems = append(problems, fmt.Sprintf("ports.aliases.%s: canonical variable name must be a string (%s)", name, err))
			continue
		}

		aliases[name] = canonical
	}

	return problems
}

func parsePortsEnv(node *yaml.Node, env map[string]int) []string {
	if node.Kind != yaml.MappingNode {
		return []string{"ports.env: must be a mapping of environment variable name to index"}
	}

	var problems []string

	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		name := keyNode.Value

		if err := validateEnvironmentVariableName(name); err != nil {
			problems = append(problems, err.Error())
			continue
		}

		var portIndex int
		if err := valueNode.Decode(&portIndex); err != nil {
			problems = append(problems, fmt.Sprintf("ports.env.%s: index must be an integer (%s)", name, err))
			continue
		}

		if portIndex < 0 {
			problems = append(problems, fmt.Sprintf("ports.env.%s: index must not be negative, got %d", name, portIndex))
			continue
		}

		env[name] = portIndex
	}

	return problems
}

// validatePortIndices enforces uniqueness and the block_size bound. Reported
// deterministically (sorted by env var name) so error output and tests are
// stable across map iteration order.
func validatePortIndices(env map[string]int, blockSize int) []string {
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)

	var problems []string
	seenIndices := map[int]string{}

	for _, name := range names {
		index := env[name]

		if index >= blockSize {
			problems = append(problems, fmt.Sprintf("ports.env.%s: index %d must be less than ports.block_size (%d)", name, index, blockSize))
		}

		if owner, taken := seenIndices[index]; taken {
			problems = append(problems, fmt.Sprintf("ports.env.%s: index %d is already used by ports.env.%s (indices must be unique)", name, index, owner))
			continue
		}
		seenIndices[index] = name
	}

	return problems
}

// validatePortAliases enforces ports.aliases' two rules: every alias's
// canonical variable must exist in ports.env, and an alias name must not
// collide with a ports.env key (a collision with a different alias key is
// structurally impossible: aliases is a map, so a repeated key in the YAML
// source simply overwrites itself during parsing, same as ports.env).
// Reported deterministically (sorted by alias name) for stable error output
// and tests.
func validatePortAliases(env map[string]int, aliases map[string]string) []string {
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	var problems []string

	for _, alias := range names {
		canonical := aliases[alias]

		if _, collides := env[alias]; collides {
			problems = append(problems, fmt.Sprintf("ports.aliases.%s: collides with ports.env.%s (alias and ports.env names must be disjoint)", alias, alias))
		}

		if _, exists := env[canonical]; !exists {
			problems = append(problems, fmt.Sprintf("ports.aliases.%s: canonical variable %q is not defined in ports.env", alias, canonical))
		}
	}

	return problems
}
