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

// repositoryContext is the resolved roots and loaded manifest governing the
// current directory. Every command that operates on a whole repository
// (create, list without --all-repos, path, remove, switch) starts here.
//
// Two distinct roots are resolved, on purpose, and must not be conflated
// (see DESIGN.md's "Manifest resolution" note):
//   - CheckoutRoot is whichever worktree actually invoked the command, and
//     is always where the manifest itself is read from.
//   - MainRepository is the repository's main worktree, unrelated to which
//     worktree invoked the command; it is where instance discovery walks
//     from and where provisioning's symlink/copy/copy_files sources live.
type repositoryContext struct {
	CheckoutRoot   string
	MainRepository string
	Manifest       *manifest.Manifest
	Warnings       []manifest.Warning
}

// loadRepositoryContext resolves dir's own checkout root and its
// repository's main worktree, then loads .treehouse.yml from the checkout
// root. Both "dir isn't inside a git repository" and "the checkout has no
// manifest" map to ExitNotTreehouseRepository, per DESIGN.md's exit code
// table.
//
// The manifest is deliberately read from CheckoutRoot, not MainRepository:
// a feature branch that adds .treehouse.yml, checked out into a `git
// worktree add` worktree, must not depend on the main worktree (which may
// still be sitting on a branch that never touched the manifest) already
// having the file. See DESIGN.md's "Manifest resolution" note.
func loadRepositoryContext(dir string) (*repositoryContext, error) {
	checkoutRoot, err := gitx.WorktreeRoot(dir)
	if err != nil {
		return nil, NewExitError(ExitNotTreehouseRepository, fmt.Errorf("not inside a git repository: %w", err))
	}

	mainRepository, err := gitx.FindMainRepository(dir)
	if err != nil {
		return nil, NewExitError(ExitNotTreehouseRepository, fmt.Errorf("not inside a git repository: %w", err))
	}

	manifestPath := filepath.Join(checkoutRoot, manifestFileName)
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, NewExitError(
			ExitNotTreehouseRepository,
			fmt.Errorf("%s has no %s; not a treehouse-enabled repository", checkoutRoot, manifestFileName),
		)
	}

	parsed, warnings, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	return &repositoryContext{
		CheckoutRoot:   checkoutRoot,
		MainRepository: mainRepository,
		Manifest:       parsed,
		Warnings:       warnings,
	}, nil
}

// loadManifestFor loads the manifest belonging to an already-known
// checkout root, for commands that resolved their instance context first
// and only need the manifest afterward (up, provision). Callers must pass
// the instance's own worktree root (inst.Path) rather than
// inst.MainRepository: once inside an instance, the instance's own
// worktree is the invoking checkout, and its manifest must be resolved the
// same way loadRepositoryContext resolves it for every other command (see
// the CheckoutRoot doc above).
func loadManifestFor(root string) (*manifest.Manifest, error) {
	parsed, _, err := manifest.Load(filepath.Join(root, manifestFileName))
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	return parsed, nil
}
