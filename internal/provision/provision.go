// Package provision materializes a fresh instance's dependencies (symlinks,
// copy-on-write clones, plain file copies) and runs the manifest's
// post-create hook, per DESIGN.md's "Copy-on-write strategy" and
// "Command behavior" sections. It never imports internal/compose: bringing
// the compose stack up is the caller's concern, so the state this package
// records only ever moves between StateWorktreeCreated, StateDepsCloned,
// StateProvisioned, and StateProvisionFailed.
package provision

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/manifest"
)

// MaterializeDependencies runs provision.symlink, provision.copy, and
// provision.copy_files in that order (DESIGN.md steps a-c), recording the
// sha256 of the main repository's lockfile for every copy entry that has a
// reconcile step. It does not itself change inst.State or save it — the
// caller advances the state to StateDepsCloned and persists it, so the two
// changes land in a single atomic env-file write.
func MaterializeDependencies(inst *instance.Instance, manifestConfig *manifest.Manifest) error {
	if err := symlinkEntries(inst.MainRepository, inst.Path, manifestConfig.Provision.Symlink); err != nil {
		return err
	}

	for _, entry := range manifestConfig.Provision.Copy {
		if err := copyEntry(inst.MainRepository, inst.Path, entry); err != nil {
			return err
		}

		if entry.Reconcile != nil {
			hash, err := hashLockfile(inst.MainRepository, entry.Reconcile.Lockfile)
			if err != nil {
				return err
			}
			inst.LockHashes[entry.Reconcile.Lockfile] = hash
		}
	}

	if err := copyFiles(inst.MainRepository, inst.Path, manifestConfig.Provision.CopyFiles); err != nil {
		return err
	}

	return nil
}

// Reconcile compares every provision.copy entry that declares a reconcile
// step against its recorded lock hash, re-running reconcile.command inside
// the instance only when the main repository's lockfile has drifted since
// the directory was cloned (or last reconciled). It runs during both the
// initial provisioning pipeline and `treehouse provision --retry`, per
// DESIGN.md's "Copy-on-write strategy" section. Like MaterializeDependencies,
// it mutates inst.LockHashes but leaves saving to the caller.
func Reconcile(inst *instance.Instance, manifestConfig *manifest.Manifest) error {
	for _, entry := range manifestConfig.Provision.Copy {
		if entry.Reconcile == nil {
			continue
		}

		currentHash, err := hashLockfile(inst.MainRepository, entry.Reconcile.Lockfile)
		if err != nil {
			return err
		}

		if currentHash == inst.LockHashes[entry.Reconcile.Lockfile] {
			continue
		}

		if err := runInDir(inst.Path, entry.Reconcile.Command); err != nil {
			return fmt.Errorf("reconcile %s (lockfile %s drifted): %w", entry.Path, entry.Reconcile.Lockfile, err)
		}

		inst.LockHashes[entry.Reconcile.Lockfile] = currentHash
	}

	return nil
}

// runInDir runs command as a shell command line with dir as its working
// directory, streaming its output. command always comes from the adopting
// repository's own committed .treehouse.yml (reconcile.command, e.g.
// "composer install" or "npm install") — a trusted, version-controlled
// config value the developer already has write access to, not external
// input — so shelling out through sh -c to support ordinary shell syntax
// is the intended contract, the same trust boundary as provision.hook.
func runInDir(dir, command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
