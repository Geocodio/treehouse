package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

var knownProvisionKeys = map[string]bool{
	"symlink":    true,
	"copy":       true,
	"copy_files": true,
	"hook":       true,
}

var knownCopyEntryKeys = map[string]bool{
	"path":      true,
	"reconcile": true,
}

var knownReconcileKeys = map[string]bool{
	"lockfile": true,
	"command":  true,
}

func parseProvision(node *yaml.Node, result *Manifest) ([]string, []Warning) {
	if node.Kind != yaml.MappingNode {
		return []string{"provision: must be a mapping"}, nil
	}

	var problems []string
	var warnings []Warning

	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]

		switch keyNode.Value {
		case "symlink":
			paths, pathProblems := parseRelativePathList("provision.symlink", valueNode)
			problems = append(problems, pathProblems...)
			result.Provision.Symlink = paths
		case "copy_files":
			paths, pathProblems := parseRelativePathList("provision.copy_files", valueNode)
			problems = append(problems, pathProblems...)
			result.Provision.CopyFiles = paths
		case "hook":
			var hook string
			if err := valueNode.Decode(&hook); err != nil {
				problems = append(problems, fmt.Sprintf("provision.hook: must be a string (%s)", err))
				break
			}
			if err := validateRelativePath("provision.hook", hook); err != nil {
				problems = append(problems, err.Error())
				break
			}
			result.Provision.Hook = hook
		case "copy":
			entries, copyProblems, copyWarnings := parseCopyEntries(valueNode)
			problems = append(problems, copyProblems...)
			warnings = append(warnings, copyWarnings...)
			result.Provision.Copy = entries
		default:
			if !knownProvisionKeys[keyNode.Value] {
				warnings = append(warnings, Warning{
					Key:     "provision." + keyNode.Value,
					Message: fmt.Sprintf("unknown key (line %d); ignored", keyNode.Line),
				})
			}
		}
	}

	return problems, warnings
}

func parseRelativePathList(key string, node *yaml.Node) ([]string, []string) {
	if node.Kind != yaml.SequenceNode {
		return nil, []string{fmt.Sprintf("%s: must be a list of paths", key)}
	}

	var problems []string
	paths := make([]string, 0, len(node.Content))

	for _, item := range node.Content {
		var path string
		if err := item.Decode(&path); err != nil {
			problems = append(problems, fmt.Sprintf("%s: each entry must be a string (%s)", key, err))
			continue
		}
		if err := validateRelativePath(key, path); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		paths = append(paths, path)
	}

	return paths, problems
}

// parseCopyEntries handles provision.copy's string-or-struct entries: a
// plain string is a path with no reconciliation, while a mapping pairs a
// path with an optional reconcile step.
func parseCopyEntries(node *yaml.Node) ([]CopyEntry, []string, []Warning) {
	if node.Kind != yaml.SequenceNode {
		return nil, []string{"provision.copy: must be a list"}, nil
	}

	var problems []string
	var warnings []Warning
	entries := make([]CopyEntry, 0, len(node.Content))

	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			var path string
			if err := item.Decode(&path); err != nil {
				problems = append(problems, fmt.Sprintf("provision.copy: entry must be a string or mapping (%s)", err))
				continue
			}
			if err := validateRelativePath("provision.copy", path); err != nil {
				problems = append(problems, err.Error())
				continue
			}
			entries = append(entries, CopyEntry{Path: path})
		case yaml.MappingNode:
			entry, entryProblems, entryWarnings := parseCopyEntryMapping(item)
			problems = append(problems, entryProblems...)
			warnings = append(warnings, entryWarnings...)
			if entry != nil {
				entries = append(entries, *entry)
			}
		default:
			problems = append(problems, "provision.copy: entry must be a string or mapping")
		}
	}

	return entries, problems, warnings
}

func parseCopyEntryMapping(node *yaml.Node) (*CopyEntry, []string, []Warning) {
	var problems []string
	var warnings []Warning
	entry := CopyEntry{}
	havePath := false

	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]

		switch keyNode.Value {
		case "path":
			havePath = true
			var path string
			if err := valueNode.Decode(&path); err != nil {
				problems = append(problems, fmt.Sprintf("provision.copy: path must be a string (%s)", err))
				break
			}
			if err := validateRelativePath("provision.copy", path); err != nil {
				problems = append(problems, err.Error())
				break
			}
			entry.Path = path
		case "reconcile":
			reconcile, reconcileProblems, reconcileWarnings := parseReconcile(valueNode, entry.Path)
			problems = append(problems, reconcileProblems...)
			warnings = append(warnings, reconcileWarnings...)
			entry.Reconcile = reconcile
		default:
			if !knownCopyEntryKeys[keyNode.Value] {
				warnings = append(warnings, Warning{
					Key:     "provision.copy." + keyNode.Value,
					Message: fmt.Sprintf("unknown key (line %d); ignored", keyNode.Line),
				})
			}
		}
	}

	if !havePath {
		problems = append(problems, "provision.copy: entry is missing required key \"path\"")
		return nil, problems, warnings
	}

	return &entry, problems, warnings
}

func parseReconcile(node *yaml.Node, copyPath string) (*ReconcileConfig, []string, []Warning) {
	if node.Kind != yaml.MappingNode {
		return nil, []string{fmt.Sprintf("provision.copy[%s].reconcile: must be a mapping", copyPath)}, nil
	}

	var problems []string
	var warnings []Warning
	reconcile := ReconcileConfig{}

	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]

		switch keyNode.Value {
		case "lockfile":
			if err := valueNode.Decode(&reconcile.Lockfile); err != nil {
				problems = append(problems, fmt.Sprintf("provision.copy[%s].reconcile.lockfile: must be a string (%s)", copyPath, err))
			}
		case "command":
			if err := valueNode.Decode(&reconcile.Command); err != nil {
				problems = append(problems, fmt.Sprintf("provision.copy[%s].reconcile.command: must be a string (%s)", copyPath, err))
			}
		default:
			if !knownReconcileKeys[keyNode.Value] {
				warnings = append(warnings, Warning{
					Key:     fmt.Sprintf("provision.copy[%s].reconcile.%s", copyPath, keyNode.Value),
					Message: fmt.Sprintf("unknown key (line %d); ignored", keyNode.Line),
				})
			}
		}
	}

	if reconcile.Lockfile == "" || reconcile.Command == "" {
		problems = append(problems, fmt.Sprintf("provision.copy[%s].reconcile: requires both \"lockfile\" and \"command\"", copyPath))
		return nil, problems, warnings
	}

	return &reconcile, problems, warnings
}
