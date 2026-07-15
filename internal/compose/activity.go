package compose

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ContainerActivityTimestamp returns the most recent StartedAt or FinishedAt
// timestamp across every container (running or stopped) belonging to
// projectName, or the zero time if the project has no containers at all.
// This is one of gc/prune's three idle-detection signals per DESIGN.md's
// "gc idle detection" rule (container activity, git HEAD reflog mtime,
// .treehouse.env mtime): a single `docker ps -a` plus a single `docker
// inspect` covers every container in the project, never one inspect call
// per container.
func ContainerActivityTimestamp(projectName string) (time.Time, error) {
	ids, err := containerIDs(projectName)
	if err != nil {
		return time.Time{}, err
	}
	if len(ids) == 0 {
		return time.Time{}, nil
	}

	args := append([]string{"inspect", "--format", "{{.State.StartedAt}}\t{{.State.FinishedAt}}"}, ids...)
	output, err := exec.Command("docker", args...).Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("running docker inspect: %w", err)
	}

	return parseContainerActivity(output), nil
}

// containerIDs returns every container ID (running or stopped) belonging to
// projectName.
func containerIDs(projectName string) ([]string, error) {
	output, err := exec.Command(
		"docker", "ps", "-a",
		"--filter", containerLabelPrefix+projectName,
		"--format", "{{.ID}}",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("running docker ps: %w", err)
	}

	var ids []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id != "" {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// dockerZeroTime is what docker prints for State.FinishedAt on a container
// that has never exited (i.e. is still running).
const dockerZeroTime = "0001-01-01T00:00:00Z"

// parseContainerActivity parses one "startedAt\tfinishedAt" line per
// container (docker's inspect --format output, RFC3339Nano timestamps) and
// returns the newest of every timestamp seen, ignoring dockerZeroTime and
// any line that fails to parse.
func parseContainerActivity(output []byte) time.Time {
	var newest time.Time

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field == "" || field == dockerZeroTime {
				continue
			}

			parsed, err := time.Parse(time.RFC3339Nano, field)
			if err != nil {
				continue
			}

			if parsed.After(newest) {
				newest = parsed
			}
		}
	}

	return newest
}
