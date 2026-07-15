package instance

import (
	"os"
	"path/filepath"

	"github.com/geocodio/treehouse/internal/gitx"
)

// Discover enumerates every treehouse instance for the repository
// containing repositoryDir, by walking `git worktree list --porcelain` and
// loading .treehouse.env from every linked worktree that has one. The main
// worktree never has an env file (only instances do), so it is skipped
// naturally rather than specially.
func Discover(repositoryDir string) ([]Instance, error) {
	entries, err := gitx.ListWorktrees(repositoryDir)
	if err != nil {
		return nil, err
	}

	var instances []Instance
	for _, entry := range entries {
		envFilePath := filepath.Join(entry.Path, EnvFileName)
		if _, err := os.Stat(envFilePath); err != nil {
			continue
		}

		loaded, err := LoadEnvFile(envFilePath)
		if err != nil {
			return nil, err
		}

		instances = append(instances, *loaded)
	}

	return instances, nil
}

// FindEnvFileUpwards walks upward from startDir looking for .treehouse.env,
// stopping at the first directory that has one (or at the filesystem
// root). Per DESIGN.md's compose passthrough rule ("inside an instance
// (cwd or ancestor has .treehouse.env)"), this intentionally does not rely
// on git at all: a plain directory walk also works from a subdirectory
// nested inside an instance's worktree.
func FindEnvFileUpwards(startDir string) (path string, found bool, err error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false, err
	}

	for {
		candidate := filepath.Join(dir, EnvFileName)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, true, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}
