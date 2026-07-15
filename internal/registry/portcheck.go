package registry

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// PortChecker reports which TCP ports currently have a listening socket
// bound on this machine. It is an interface so allocation logic can be
// tested without real sockets or a real lsof binary; LsofPortChecker is the
// production implementation.
type PortChecker interface {
	ListeningPorts() (map[int]bool, error)
}

// LsofPortChecker shells out to lsof exactly once per call and parses its
// output, per DESIGN.md's "single invocation, parsed once — never one lsof
// per port" rule.
type LsofPortChecker struct{}

// listenLinePattern matches the trailing "<host>:<port> (LISTEN)" that lsof
// prints for every listening socket, e.g. "*:4640 (LISTEN)",
// "127.0.0.1:5432 (LISTEN)", or "[::1]:4640 (LISTEN)". Anchoring on this
// suffix rather than counting columns tolerates the header row and any
// COMMAND/USER values that happen to contain whitespace.
var listenLinePattern = regexp.MustCompile(`:(\d+)\s+\(LISTEN\)\s*$`)

// ListeningPorts runs `lsof -nP -iTCP -sTCP:LISTEN` and returns the set of
// locally bound listening TCP ports.
func (LsofPortChecker) ListeningPorts() (map[int]bool, error) {
	output, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(bytes.TrimSpace(output)) == 0 {
			// lsof exits non-zero when it finds nothing matching the
			// filter; that means no listening ports, not a failure.
			return map[int]bool{}, nil
		}

		return nil, fmt.Errorf("running lsof: %w", err)
	}

	return parseLsofListeningPorts(output), nil
}

func parseLsofListeningPorts(output []byte) map[int]bool {
	ports := map[int]bool{}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		matches := listenLinePattern.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}

		port, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		ports[port] = true
	}

	return ports
}

// ListeningPortProcesses runs lsof once and maps each listening TCP port to
// the name of the process bound to it (lsof's COMMAND column), best-effort:
// a port lsof can't attribute to a command, or lsof being unavailable
// entirely, simply yields an empty or partial map rather than an error, so
// callers like compose.VerifyPortBlockFree can name a squatting process
// "when identifiable" without that enrichment ever blocking the caller.
func ListeningPortProcesses() (map[int]string, error) {
	output, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(bytes.TrimSpace(output)) == 0 {
			// Same "nothing listening" case as ListeningPorts.
			return map[int]string{}, nil
		}

		return nil, fmt.Errorf("running lsof: %w", err)
	}

	return parseLsofProcesses(output), nil
}

// parseLsofProcesses reuses listenLinePattern to identify genuine LISTEN
// lines (skipping the header row and anything else), then takes the line's
// first whitespace-separated field as the COMMAND column.
func parseLsofProcesses(output []byte) map[int]string {
	processes := map[int]string{}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		matches := listenLinePattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		port, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		processes[port] = fields[0]
	}

	return processes
}
