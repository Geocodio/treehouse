package provision

import (
	"fmt"
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
func copyEntry(mainRepository, instancePath string, entry manifest.CopyEntry) error {
	source := filepath.Join(mainRepository, entry.Path)
	destination := filepath.Join(instancePath, entry.Path)

	if _, err := os.Stat(destination); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("provision.copy %s: %w", entry.Path, err)
	}

	if err := copyOnWrite(source, destination); err != nil {
		return fmt.Errorf("provision.copy %s: %w", entry.Path, err)
	}

	return nil
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
// previously copied file survive `provision --retry`.
func copyFiles(mainRepository, instancePath string, files []string) error {
	for _, relativePath := range files {
		source := filepath.Join(mainRepository, relativePath)
		destination := filepath.Join(instancePath, relativePath)

		if _, err := os.Stat(destination); err == nil {
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
