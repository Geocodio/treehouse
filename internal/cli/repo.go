package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geocodio/treehouse/internal/gitx"
	"github.com/geocodio/treehouse/internal/manifest"
)

// manifestFileName is the manifest's file name at a repository's root, per
// DESIGN.md's "Files and locations" table.
const manifestFileName = ".treehouse.yml"

// repositoryContext is the resolved main repository and loaded manifest
// governing the current directory. Every command that operates on a whole
// repository (create, list without --all-repos, path, remove) starts here.
type repositoryContext struct {
	MainRepository string
	Manifest       *manifest.Manifest
	Warnings       []manifest.Warning
}

// loadRepositoryContext resolves dir's main git repository and loads its
// .treehouse.yml. Both "dir isn't inside a git repository" and "the
// repository has no manifest" map to ExitNotTreehouseRepository, per
// DESIGN.md's exit code table.
func loadRepositoryContext(dir string) (*repositoryContext, error) {
	mainRepository, err := gitx.FindMainRepository(dir)
	if err != nil {
		return nil, NewExitError(ExitNotTreehouseRepository, fmt.Errorf("not inside a git repository: %w", err))
	}

	manifestPath := filepath.Join(mainRepository, manifestFileName)
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, NewExitError(
			ExitNotTreehouseRepository,
			fmt.Errorf("%s has no %s; not a treehouse-enabled repository", mainRepository, manifestFileName),
		)
	}

	parsed, warnings, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	return &repositoryContext{MainRepository: mainRepository, Manifest: parsed, Warnings: warnings}, nil
}

// loadManifestFor loads the manifest belonging to an already-known main
// repository path (e.g. an instance's TREEHOUSE_MAIN_REPOSITORY), for
// commands that resolved their instance context first and only need the
// manifest afterward (up, provision).
func loadManifestFor(mainRepository string) (*manifest.Manifest, error) {
	parsed, _, err := manifest.Load(filepath.Join(mainRepository, manifestFileName))
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	return parsed, nil
}
