package provision

import (
	"fmt"
	"os"
	"path/filepath"
)

// symlinkEntries symlinks every provision.symlink entry to the main
// repository's copy. Per DESIGN.md: replace the destination only if it is
// already a symlink; refuse (error) if a non-empty real directory sits
// there instead — that would silently discard instance-local content the
// developer may not have meant to share.
func symlinkEntries(mainRepository, instancePath string, entries []string) error {
	for _, relativePath := range entries {
		target := filepath.Join(mainRepository, relativePath)
		link := filepath.Join(instancePath, relativePath)

		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			return fmt.Errorf("provision.symlink %s: %w", relativePath, err)
		}

		if err := prepareSymlinkDestination(relativePath, link); err != nil {
			return err
		}

		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("provision.symlink %s: %w", relativePath, err)
		}
	}

	return nil
}

func prepareSymlinkDestination(relativePath, link string) error {
	info, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("provision.symlink %s: %w", relativePath, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("provision.symlink %s: replacing existing symlink: %w", relativePath, err)
		}
		return nil
	}

	if info.IsDir() {
		contents, err := os.ReadDir(link)
		if err != nil {
			return fmt.Errorf("provision.symlink %s: %w", relativePath, err)
		}
		if len(contents) > 0 {
			return fmt.Errorf(
				"provision.symlink %s: refusing to replace non-empty directory %s with a symlink",
				relativePath, link,
			)
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("provision.symlink %s: removing empty directory: %w", relativePath, err)
		}
		return nil
	}

	return fmt.Errorf("provision.symlink %s: refusing to replace existing file %s with a symlink", relativePath, link)
}
