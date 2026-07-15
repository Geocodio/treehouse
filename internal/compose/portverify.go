package compose

import (
	"fmt"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

// VerifyPortBlockFree checks every port in inst's claimed block
// [inst.BlockStart, inst.BlockStart+blockSize) against currently listening
// sockets, reusing registry.LsofPortChecker per DESIGN.md's "up re
// -verifies the port block against listening sockets first (reuse registry
// port checker)" rule. It names the specific conflicting port (and its
// manifest env var name, if any) on failure. Consistent with the
// registry's own degrade-not-fail behavior, an unavailable lsof produces no
// error rather than blocking `up`.
func VerifyPortBlockFree(inst *instance.Instance, blockSize int) error {
	return verifyPortBlockFree(registry.LsofPortChecker{}, inst, blockSize)
}

// verifyPortBlockFree is VerifyPortBlockFree with an injectable
// registry.PortChecker, so the conflict-detection logic is unit-testable
// without a real lsof binary or real listening sockets.
func verifyPortBlockFree(checker registry.PortChecker, inst *instance.Instance, blockSize int) error {
	listening, err := checker.ListeningPorts()
	if err != nil {
		return nil
	}

	portNames := make(map[int]string, len(inst.Ports))
	for name, port := range inst.Ports {
		portNames[port] = name
	}

	for port := inst.BlockStart; port < inst.BlockStart+blockSize; port++ {
		if !listening[port] {
			continue
		}

		if name, ok := portNames[port]; ok {
			return fmt.Errorf(
				"port %d (%s) is already in use; refusing to start %s",
				port, name, inst.ComposeProject,
			)
		}
		return fmt.Errorf("port %d is already in use; refusing to start %s", port, inst.ComposeProject)
	}

	return nil
}
