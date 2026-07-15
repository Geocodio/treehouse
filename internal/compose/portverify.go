package compose

import (
	"fmt"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

// processLookup resolves listening-port -> process name for
// portConflictError's optional enrichment. registry.ListeningPortProcesses
// is the production implementation; verifyPortBlockFree takes it as a
// parameter (rather than calling the package function directly) so tests
// can stub it out instead of shelling out to a real lsof.
type processLookup func() (map[int]string, error)

// VerifyPortBlockFree checks every port in inst's claimed block
// [inst.BlockStart, inst.BlockStart+blockSize) against currently listening
// sockets, reusing registry.LsofPortChecker per DESIGN.md's "up re
// -verifies the port block against listening sockets" rule. Ports already
// published by inst's own compose project (queried via ProjectPublishedPorts)
// are excluded from the comparison first, so a stack that's only partially
// up never collides with itself; this is what makes it safe to run this
// check unconditionally rather than only when the project has zero running
// containers. It names the specific conflicting port, its manifest env var
// name (if any), and the squatting process (if lsof can identify it) on
// failure. Consistent with the registry's own degrade-not-fail behavior, an
// unavailable lsof produces no error rather than blocking `up`.
func VerifyPortBlockFree(inst *instance.Instance, blockSize int) error {
	// Best-effort: a docker ps failure here (e.g. the project has no
	// containers at all yet) must not block `up` — it just means there are
	// no ports to exclude as "already ours".
	ownPorts, err := ProjectPublishedPorts(inst.ComposeProject)
	if err != nil {
		ownPorts = map[int]bool{}
	}

	return verifyPortBlockFree(registry.LsofPortChecker{}, ownPorts, registry.ListeningPortProcesses, inst, blockSize)
}

// verifyPortBlockFree is VerifyPortBlockFree with an injectable
// registry.PortChecker, ownPorts set, and process lookup, so the conflict
// -detection logic is unit-testable without a real lsof or docker binary.
func verifyPortBlockFree(checker registry.PortChecker, ownPorts map[int]bool, lookupProcesses processLookup, inst *instance.Instance, blockSize int) error {
	listening, err := checker.ListeningPorts()
	if err != nil {
		return nil
	}

	portNames := make(map[int]string, len(inst.Ports))
	for name, port := range inst.Ports {
		portNames[port] = name
	}

	// Best-effort process-name enrichment; a failure here must not block
	// `up` either, it just means the error can't name the squatting
	// process.
	processes, _ := lookupProcesses()

	for port := inst.BlockStart; port < inst.BlockStart+blockSize; port++ {
		if !listening[port] || ownPorts[port] {
			continue
		}

		return portConflictError(port, portNames[port], processes[port], inst.ComposeProject)
	}

	return nil
}

// portConflictError builds up's port-conflict error, naming the manifest env
// var and squatting process only when known.
func portConflictError(port int, envVarName, process, composeProject string) error {
	description := fmt.Sprintf("port %d", port)
	if envVarName != "" {
		description += fmt.Sprintf(" (%s)", envVarName)
	}
	if process != "" {
		description += fmt.Sprintf(", held by %s", process)
	}

	return fmt.Errorf("%s is already in use; refusing to start %s", description, composeProject)
}
