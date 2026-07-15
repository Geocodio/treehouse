package provision

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/geocodio/treehouse/internal/manifest"
)

// copyEntry copy-on-write clones one provision.copy entry from the main
// repository into the instance. It is idempotent: if the destination
// already exists (a previous MaterializeDependencies run, or a
// `provision --retry` resuming past StateWorktreeCreated), it is left
// untouched rather than re-cloned.
//
// A source that doesn't exist in the main repository at all is not a
// failure: it reports skipped=true after printing a warning to warnings,
// rather than erroring out. A generated build-artifact directory
// (vendor/, node_modules/, public/build) that the main repository has
// simply never built locally is a routine state during real-world
// adoption, not a misconfiguration — unlike a genuinely broken manifest
// entry, it shouldn't fail the whole create/provision run. Callers with a
// `reconcile` step must treat a skip specially (see MaterializeDependencies):
// there is no cloned directory yet for a lockfile-hash comparison to mean
// anything, so the installer has to run unconditionally instead.
func copyEntry(mainRepository, instancePath string, entry manifest.CopyEntry, warnings io.Writer) (skipped bool, err error) {
	source := filepath.Join(mainRepository, entry.Path)
	destination := filepath.Join(instancePath, entry.Path)

	if _, statErr := os.Stat(destination); statErr == nil {
		return false, nil
	}

	if _, statErr := os.Stat(source); statErr != nil {
		if !os.IsNotExist(statErr) {
			return false, fmt.Errorf("provision.copy %s: %w", entry.Path, statErr)
		}
		fmt.Fprintf(warnings, "warning: skipping %s: not present in main repository\n", entry.Path)
		return true, nil
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(destination), 0o755); mkdirErr != nil {
		return false, fmt.Errorf("provision.copy %s: %w", entry.Path, mkdirErr)
	}

	if cloneErr := copyOnWrite(source, destination); cloneErr != nil {
		return false, fmt.Errorf("provision.copy %s: %w", entry.Path, cloneErr)
	}

	return false, nil
}

// copyOnWrite clones source to destination using cp's fast-path clone
// flags rather than a Go file-walk, per DESIGN.md's "Copy-on-write
// strategy": macOS/APFS uses `cp -Rc` (clonefile), falling back to a plain
// `cp -R` when the volume doesn't support it (e.g. a non-APFS external
// disk); Linux uses `cp -a --reflink=auto`, which degrades to a full copy
// automatically on filesystems without reflink support (e.g. ext4).
func copyOnWrite(source, destination string) error {
	if runtime.GOOS == "darwin" {
		if err := runCp("-Rc", source, destination); err == nil {
			return nil
		}
		return runCp("-R", source, destination)
	}

	return runCp("-a", "--reflink=auto", source, destination)
}

func runCp(args ...string) error {
	command := exec.Command("cp", args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

// copyFiles plainly copies each provision.copy_files entry (small
// configuration files like .env.docker, never CoW-cloned since they're not
// large directories). Like copyEntry, it is idempotent: an existing
// destination file is left alone so a developer's local edits to a
// previously copied file survive `provision --retry`. A source that
// doesn't exist in the main repository is skipped with a warning rather
// than failing, for the same "generated file the main repo hasn't built
// yet" reason as copyEntry — copy_files has no reconcile concept, so there
// is nothing further to force here.
func copyFiles(mainRepository, instancePath string, files []string, warnings io.Writer) error {
	for _, relativePath := range files {
		source := filepath.Join(mainRepository, relativePath)
		destination := filepath.Join(instancePath, relativePath)

		if _, err := os.Stat(destination); err == nil {
			continue
		}

		if _, statErr := os.Stat(source); statErr != nil {
			if !os.IsNotExist(statErr) {
				return fmt.Errorf("provision.copy_files %s: %w", relativePath, statErr)
			}
			fmt.Fprintf(warnings, "warning: skipping %s: not present in main repository\n", relativePath)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("provision.copy_files %s: %w", relativePath, err)
		}

		if err := runCp(source, destination); err != nil {
			return fmt.Errorf("provision.copy_files %s: %w", relativePath, err)
		}
	}

	return nil
}
