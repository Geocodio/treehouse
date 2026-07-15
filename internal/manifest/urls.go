package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

var knownURLEntryKeys = map[string]bool{
	"name": true,
	"url":  true,
}

func parseURLs(node *yaml.Node, result *Manifest) ([]string, []Warning) {
	if node.Kind != yaml.SequenceNode {
		return []string{"urls: must be a list"}, nil
	}

	var problems []string
	var warnings []Warning
	entries := make([]URL, 0, len(node.Content))

	for position, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			problems = append(problems, fmt.Sprintf("urls[%d]: must be a mapping with \"name\" and \"url\"", position))
			continue
		}

		entry := URL{}
		haveName := false
		haveURL := false

		for index := 0; index+1 < len(item.Content); index += 2 {
			keyNode := item.Content[index]
			valueNode := item.Content[index+1]

			switch keyNode.Value {
			case "name":
				haveName = true
				if err := valueNode.Decode(&entry.Name); err != nil {
					problems = append(problems, fmt.Sprintf("urls[%d].name: must be a string (%s)", position, err))
				}
			case "url":
				haveURL = true
				if err := valueNode.Decode(&entry.URL); err != nil {
					problems = append(problems, fmt.Sprintf("urls[%d].url: must be a string (%s)", position, err))
				}
			default:
				if !knownURLEntryKeys[keyNode.Value] {
					warnings = append(warnings, Warning{
						Key:     fmt.Sprintf("urls[%d].%s", position, keyNode.Value),
						Message: fmt.Sprintf("unknown key (line %d); ignored", keyNode.Line),
					})
				}
			}
		}

		if !haveName {
			problems = append(problems, fmt.Sprintf("urls[%d]: is missing required key \"name\"", position))
		}
		if !haveURL {
			problems = append(problems, fmt.Sprintf("urls[%d]: is missing required key \"url\"", position))
		}

		if haveName && haveURL {
			entries = append(entries, entry)
		}
	}

	result.URLs = entries

	return problems, warnings
}
