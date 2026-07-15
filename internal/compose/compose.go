// Package compose builds and runs docker compose invocations, injecting an
// instance's env-file and project name when appropriate and querying
// container status exclusively by the com.docker.compose.project label —
// never by container name substring — per DESIGN.md's "Command behavior"
// section.
package compose

import (
	"errors"
	"io"
	"os/exec"

	"github.com/geocodio/treehouse/internal/instance"
)

// ErrDockerUnavailable is returned by NewRunner when neither the `docker
// compose` plugin nor the `docker-compose` fallback can be invoked.
var ErrDockerUnavailable = errors.New("compose: docker (or docker-compose) is not available")

// Runner invokes docker compose (or its docker-compose fallback), detected
// once at construction time per DESIGN.md's "Detect docker compose plugin
// vs docker-compose fallback once" rule.
type Runner struct {
	// binary is the base command and any leading subcommand: ["docker",
	// "compose"] for the plugin, or ["docker-compose"] for the fallback.
	binary []string
}

// NewRunner detects which compose invocation is available: the `docker
// compose` plugin is preferred, falling back to the standalone
// `docker-compose` binary. It returns ErrDockerUnavailable if neither
// works, which callers map to cli.ExitDockerUnavailable.
func NewRunner() (*Runner, error) {
	if commandWorks("docker", "compose", "version") {
		return &Runner{binary: []string{"docker", "compose"}}, nil
	}

	if commandWorks("docker-compose", "version") {
		return &Runner{binary: []string{"docker-compose"}}, nil
	}

	return nil, ErrDockerUnavailable
}

func commandWorks(name string, args ...string) bool {
	if _, err := exec.LookPath(name); err != nil {
		return false
	}

	return exec.Command(name, args...).Run() == nil
}

// buildArgs assembles the full docker/docker-compose argument list for
// extra, injecting --env-file and --project-name when inst is non-nil (the
// "inside an instance" case) and passing extra through unchanged when inst
// is nil (the "in a main repo" case). It is a pure function, independent of
// binary detection or process execution, so the injection rule is
// unit-testable without a real docker binary.
func buildArgs(binary []string, inst *instance.Instance, extra []string) []string {
	args := append([]string{}, binary[1:]...)

	if inst != nil {
		args = append(args, "--env-file", inst.EnvFilePath(), "--project-name", inst.ComposeProject)
	}

	return append(args, extra...)
}

func (runner *Runner) run(inst *instance.Instance, extra []string, stdout, stderr io.Writer) error {
	args := buildArgs(runner.binary, inst, extra)

	command := exec.Command(runner.binary[0], args...)
	if inst != nil {
		command.Dir = inst.Path
	}
	command.Stdout = stdout
	command.Stderr = stderr

	return command.Run()
}

// Compose is the raw passthrough used by `treehouse compose <args...>`:
// inst is nil for a main-repository invocation (args pass through
// unchanged) or the loaded instance for an instance-context invocation
// (--env-file/--project-name injected).
func (runner *Runner) Compose(inst *instance.Instance, args []string, stdout, stderr io.Writer) error {
	return runner.run(inst, args, stdout, stderr)
}

// Exec is sugar for `treehouse compose exec <service> <command...>`.
func (runner *Runner) Exec(inst *instance.Instance, service string, command []string, stdout, stderr io.Writer) error {
	args := append([]string{"exec", service}, command...)
	return runner.run(inst, args, stdout, stderr)
}

// Stop stops inst's containers without removing them.
func (runner *Runner) Stop(inst *instance.Instance, stdout, stderr io.Writer) error {
	return runner.run(inst, []string{"stop"}, stdout, stderr)
}

// Down stops and removes inst's containers and networks, additionally
// removing named volumes when removeVolumes is true.
func (runner *Runner) Down(inst *instance.Instance, removeVolumes bool, stdout, stderr io.Writer) error {
	args := []string{"down"}
	if removeVolumes {
		args = append(args, "--volumes")
	}
	return runner.run(inst, args, stdout, stderr)
}

// Restart restarts inst's containers.
func (runner *Runner) Restart(inst *instance.Instance, stdout, stderr io.Writer) error {
	return runner.run(inst, []string{"restart"}, stdout, stderr)
}

// Up brings inst's stack up in the background. If the stack has no
// containers currently running, it first re-verifies the instance's port
// block against listening sockets (see VerifyPortBlockFree), refusing to
// start rather than handing docker compose a port some other process has
// since taken.
func (runner *Runner) Up(inst *instance.Instance, blockSize int, stdout, stderr io.Writer) error {
	count, err := RunningContainerCount(inst.ComposeProject)
	if err == nil && count == 0 {
		if err := VerifyPortBlockFree(inst, blockSize); err != nil {
			return err
		}
	}

	return runner.run(inst, []string{"up", "-d"}, stdout, stderr)
}
