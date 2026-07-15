// Package cli holds command implementations, TTY detection, and JSON output
// helpers shared across treehouse's cobra commands.
package cli

// ExitCode is treehouse's contractual process exit status. These values are
// documented in the README and are part of the tool's agent-mode contract:
// scripts and automation depend on their exact numeric meaning, so existing
// values must never be renumbered or repurposed.
type ExitCode int

const (
	// ExitSuccess indicates the command completed without error.
	ExitSuccess ExitCode = 0

	// ExitGenericFailure covers any failure that does not have a more
	// specific exit code below.
	ExitGenericFailure ExitCode = 1

	// ExitPortPoolExhausted indicates the registry scanned the entire
	// configured port pool without finding a free block.
	ExitPortPoolExhausted ExitCode = 2

	// ExitDirtyTreeRefusal indicates an operation refused to discard
	// uncommitted changes in a worktree without an explicit --force.
	ExitDirtyTreeRefusal ExitCode = 3

	// ExitProvisionFailed indicates a provisioning step (clone, hook,
	// reconcile) failed; the instance is left inspectable in
	// provision-failed state.
	ExitProvisionFailed ExitCode = 4

	// ExitNotTreehouseRepository indicates the current directory is not
	// inside a repository with a .treehouse.yml manifest.
	ExitNotTreehouseRepository ExitCode = 5

	// ExitDockerUnavailable indicates docker (or the compose plugin) is
	// not installed or not reachable.
	ExitDockerUnavailable ExitCode = 6

	// ExitWouldPrompt indicates a non-interactive invocation reached a
	// point that would otherwise have prompted the user.
	ExitWouldPrompt ExitCode = 7
)
