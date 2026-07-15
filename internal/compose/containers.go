package compose

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Container is one running container reported by `docker ps`.
type Container struct {
	ID    string
	Name  string
	State string
}

// containerLabelPrefix is the docker compose label every container carries
// with its compose project name. Container queries must filter on this
// label exclusively — never match container names by substring — per
// DESIGN.md's "Command behavior" section.
const containerLabelPrefix = "label=com.docker.compose.project="

// RunningContainers returns every currently running container labeled with
// the given compose project name, via a single `docker ps --filter`
// invocation.
func RunningContainers(projectName string) ([]Container, error) {
	output, err := exec.Command(
		"docker", "ps",
		"--filter", containerLabelPrefix+projectName,
		"--format", "{{.ID}}\t{{.Names}}\t{{.State}}",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("running docker ps: %w", err)
	}

	return parseContainerLines(output), nil
}

// RunningContainerCount is a convenience wrapper around RunningContainers
// for callers that only need the count (e.g. `up`'s idempotency check and
// `status`).
func RunningContainerCount(projectName string) (int, error) {
	containers, err := RunningContainers(projectName)
	if err != nil {
		return 0, err
	}

	return len(containers), nil
}

func parseContainerLines(output []byte) []Container {
	var containers []Container

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 3 {
			continue
		}

		containers = append(containers, Container{ID: fields[0], Name: fields[1], State: fields[2]})
	}

	return containers
}
