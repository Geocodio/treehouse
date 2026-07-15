package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// knownTopLevelKeys are the manifest keys wave 1's schema understands.
// Encountering any other top-level key is a fatal error (typo protection);
// see DESIGN.md's validation rules.
var knownTopLevelKeys = map[string]bool{
	"version":   true,
	"project":   true,
	"ports":     true,
	"provision": true,
	"urls":      true,
	"location":  true,
}

// Load reads and validates the manifest at path. Fatal problems (invalid
// version, malformed values, path escapes, unknown top-level keys, etc.) are
// returned as a single *ValidationError aggregating every problem found.
// Non-fatal problems (unknown nested keys) are returned as warnings
// alongside a manifest that has otherwise been fully parsed, per DESIGN.md's
// "unknown top-level keys are an error, unknown nested keys a warning" rule.
func Load(path string) (*Manifest, []Warning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}

	if len(document.Content) == 0 {
		return nil, nil, fmt.Errorf("manifest %s is empty", path)
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("manifest %s: top level must be a mapping of keys to values", path)
	}

	result := &Manifest{
		Ports: PortsConfig{
			BlockSize: DefaultBlockSize,
			Env:       map[string]int{},
		},
	}

	var problems []string
	var warnings []Warning
	haveVersion := false
	haveProject := false

	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]

		switch keyNode.Value {
		case "version":
			haveVersion = true
			if err := parseVersion(valueNode, result); err != nil {
				problems = append(problems, err.Error())
			}
		case "project":
			haveProject = true
			var project string
			if err := valueNode.Decode(&project); err != nil {
				problems = append(problems, fmt.Sprintf("project: must be a string (%s)", err))
				break
			}
			if err := validateProjectName(project); err != nil {
				problems = append(problems, err.Error())
				break
			}
			result.Project = project
		case "ports":
			portProblems, portWarnings := parsePorts(valueNode, result)
			problems = append(problems, portProblems...)
			warnings = append(warnings, portWarnings...)
		case "provision":
			provisionProblems, provisionWarnings := parseProvision(valueNode, result)
			problems = append(problems, provisionProblems...)
			warnings = append(warnings, provisionWarnings...)
		case "urls":
			urlProblems, urlWarnings := parseURLs(valueNode, result)
			problems = append(problems, urlProblems...)
			warnings = append(warnings, urlWarnings...)
		case "location":
			var location string
			if err := valueNode.Decode(&location); err != nil {
				problems = append(problems, fmt.Sprintf("location: must be a string (%s)", err))
				break
			}
			result.Location = location
		default:
			if !knownTopLevelKeys[keyNode.Value] {
				problems = append(problems, fmt.Sprintf("unknown top-level key %q (line %d)", keyNode.Value, keyNode.Line))
			}
		}
	}

	if !haveVersion {
		problems = append(problems, "version: is required")
	}
	if !haveProject {
		problems = append(problems, "project: is required")
	}

	if len(problems) > 0 {
		return nil, warnings, &ValidationError{Problems: problems}
	}

	return result, warnings, nil
}

func parseVersion(node *yaml.Node, result *Manifest) error {
	var version int
	if err := node.Decode(&version); err != nil {
		return fmt.Errorf("version: must be an integer (%s)", err)
	}

	if version < 1 {
		return fmt.Errorf("version: must be at least 1, got %d", version)
	}

	if version > CurrentVersion {
		return fmt.Errorf("version: %d is newer than the highest version this build of treehouse supports (%d); upgrade treehouse", version, CurrentVersion)
	}

	result.Version = version

	return nil
}
