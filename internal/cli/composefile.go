package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// This file parses just enough of docker-compose.yml (via yaml.v3's node
// API, never docker itself — no docker dependency, per DESIGN.md) to power
// both `init` (propose a manifest from published ports) and `doctor`
// (compose contract checks: hardcoded published ports, container_name,
// absolute bind mounts). Parsing through yaml.Node rather than a
// map-shaped struct preserves the file's own service/port declaration
// order, which init needs for deterministic, readable output.
//
// Only the short string form of ports/volumes entries is understood; the
// long mapping form (`target:`/`published:`/`protocol:` keys, or
// `type:`/`source:`/`target:` for volumes) is out of scope for v1 and is
// silently skipped rather than failing the whole parse.

// composeServiceEntry is one service's relevant fields, in the order they
// appear in the compose file.
type composeServiceEntry struct {
	Name          string
	Ports         []string
	ContainerName string
	Volumes       []string
}

// parseComposeServices parses a docker-compose.yml's top-level "services"
// mapping into an ordered list of composeServiceEntry.
func parseComposeServices(data []byte) ([]composeServiceEntry, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parsing compose file: %w", err)
	}
	if len(document.Content) == 0 {
		return nil, fmt.Errorf("compose file is empty")
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("compose file: top level must be a mapping")
	}

	servicesNode := findMappingValue(root, "services")
	if servicesNode == nil || servicesNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf(`compose file has no top-level "services" mapping`)
	}

	var services []composeServiceEntry
	for index := 0; index+1 < len(servicesNode.Content); index += 2 {
		name := servicesNode.Content[index].Value
		serviceNode := servicesNode.Content[index+1]
		if serviceNode.Kind != yaml.MappingNode {
			continue
		}

		services = append(services, parseComposeService(name, serviceNode))
	}

	return services, nil
}

func parseComposeService(name string, node *yaml.Node) composeServiceEntry {
	entry := composeServiceEntry{Name: name}

	if portsNode := findMappingValue(node, "ports"); portsNode != nil && portsNode.Kind == yaml.SequenceNode {
		for _, item := range portsNode.Content {
			if item.Kind == yaml.ScalarNode {
				entry.Ports = append(entry.Ports, item.Value)
			}
		}
	}

	if containerNameNode := findMappingValue(node, "container_name"); containerNameNode != nil && containerNameNode.Kind == yaml.ScalarNode {
		entry.ContainerName = containerNameNode.Value
	}

	if volumesNode := findMappingValue(node, "volumes"); volumesNode != nil && volumesNode.Kind == yaml.SequenceNode {
		for _, item := range volumesNode.Content {
			if item.Kind == yaml.ScalarNode {
				entry.Volumes = append(entry.Volumes, item.Value)
			}
		}
	}

	return entry
}

// findMappingValue returns the value node paired with key inside mapping
// node, or nil if node isn't a mapping or has no such key.
func findMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}

	return nil
}

// publishedPort is one ports: entry that publishes a host port, parsed from
// its short-syntax string form.
type publishedPort struct {
	ServiceName string
	Raw         string

	// ExistingVar/ExistingDefault are set when Raw already uses
	// "${VAR:-default}" (or bare "${VAR}") substitution: init adopts VAR
	// as-is rather than proposing a new name.
	ExistingVar     string
	ExistingDefault string

	// HardcodedHost is set when Raw's host port is a plain number: init
	// proposes a var name and the compose diff needed to parameterize it.
	HardcodedHost string
}

// substitutionPattern matches "${VAR}" or "${VAR:-default}" (also accepting
// the bash "${VAR:?message}"/"${VAR-default}" separator forms loosely, by
// treating anything after the first "-" or ":" as the default/message).
var substitutionPattern = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)(?:[:-][?-]?(.*))?\}$`)

// parsePortEntry parses a single ports: short-syntax entry for serviceName,
// returning nil for a form init/doctor don't manage: a bare container port
// with no host mapping (compose would assign a random host port, which
// treehouse's fixed-port-block model has no fixed value to name), or an
// unrecognized substitution/host form.
func parsePortEntry(serviceName, raw string) *publishedPort {
	spec := raw
	if slashIndex := strings.LastIndex(spec, "/"); slashIndex != -1 {
		spec = spec[:slashIndex]
	}

	// A "${VAR:-default}" substitution's own ":-" separator must not be
	// mistaken for the host:container delimiter by a plain colon split
	// below, so it's handled as its own case first.
	if strings.HasPrefix(spec, "${") {
		return parseSubstitutedPortEntry(serviceName, raw, spec)
	}

	parts := strings.Split(spec, ":")

	var hostPart string
	switch len(parts) {
	case 1:
		return nil // bare container port: no fixed host port to manage
	case 2:
		hostPart = parts[0]
	case 3:
		hostPart = parts[1] // parts[0] is a bind IP; ignored
	default:
		return nil
	}

	if _, err := strconv.Atoi(hostPart); err == nil {
		return &publishedPort{ServiceName: serviceName, Raw: raw, HardcodedHost: hostPart}
	}

	return nil
}

// parseSubstitutedPortEntry handles a ports: entry whose host part is a
// "${VAR}" or "${VAR:-default}" substitution: spec is raw with any
// trailing "/protocol" already stripped, still starting with "${".
func parseSubstitutedPortEntry(serviceName, raw, spec string) *publishedPort {
	closeBrace := strings.Index(spec, "}")
	if closeBrace == -1 {
		return nil
	}

	hostPart := spec[:closeBrace+1]
	rest := spec[closeBrace+1:]
	if !strings.HasPrefix(rest, ":") || len(rest) < 2 {
		return nil // no container port after the substitution
	}

	matches := substitutionPattern.FindStringSubmatch(hostPart)
	if matches == nil {
		return nil
	}

	return &publishedPort{ServiceName: serviceName, Raw: raw, ExistingVar: matches[1], ExistingDefault: matches[2]}
}
