package provision

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/manifest"
)

// RunHook executes the manifest's provision.hook script (DESIGN.md step d),
// checked out at the same relative path inside the instance's own worktree
// as it is in the main repository, with the instance's full environment
// exported plus TREEHOUSE_MAIN_REPOSITORY (already part of that
// environment) and TREEHOUSE_INSTANCE_PATH. Both the hook's stdout and its
// stderr stream to this process's stderr — never its stdout, which
// DESIGN.md's "Agent mode" section reserves exclusively for `--json`'s
// single data document. That holds in every invocation, not only when
// --json is passed: stdout is a data channel, period. A chatty hook (e.g.
// one running database migrations) would otherwise interleave arbitrary
// text into `treehouse create --json`'s stdout and corrupt it. A missing
// provision.hook is not an error — the hook is optional.
//
// The caller is responsible for mapping a returned error to
// cli.ExitProvisionFailed (exit code 4) and recording StateProvisionFailed;
// this package deliberately does not import internal/cli to avoid a
// dependency cycle.
func RunHook(inst *instance.Instance, manifestConfig *manifest.Manifest) error {
	if manifestConfig.Provision.Hook == "" {
		return nil
	}

	hookPath := filepath.Join(inst.Path, manifestConfig.Provision.Hook)

	command := exec.Command(hookPath)
	command.Dir = inst.Path
	command.Env = append(os.Environ(), inst.EnvPairs()...)
	command.Env = append(command.Env, "TREEHOUSE_INSTANCE_PATH="+inst.Path)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("provision hook %s failed: %w", manifestConfig.Provision.Hook, err)
	}

	return nil
}
