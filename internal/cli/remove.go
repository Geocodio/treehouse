package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/gitx"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

func newRemoveCommand() *cobra.Command {
	var force, keepVolumes bool

	command := &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove an instance",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runRemove(cmd, name, force, keepVolumes)
		},
	}

	command.Flags().BoolVar(&force, "force", false, "remove a dirty worktree and skip the confirmation prompt")
	command.Flags().BoolVar(&keepVolumes, "keep-volumes", false, "don't remove the compose stack's named volumes")

	return command
}

func runRemove(cmd *cobra.Command, name string, force, keepVolumes bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	repo, err := loadRepositoryContext(cwd)
	if err != nil {
		return err
	}

	target, err := resolveRemoveTarget(cwd, repo.MainRepository, name, force)
	if err != nil {
		return err
	}
	if target == nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "aborted")
		return nil
	}

	stopInstanceStack(cmd, target, keepVolumes)

	if err := removeWorktree(cmd, repo.MainRepository, target, force); err != nil {
		return err
	}

	if err := releaseClaim(target); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: releasing port claim:", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "removed instance %q\n", target.Name)
	return nil
}

// resolveRemoveTarget finds the instance to remove: by name if given, or
// by cwd if it's inside one. The no-name case additionally enforces
// DESIGN.md's confirmation rule: prompt only on a TTY, otherwise require
// --force (ExitWouldPrompt).
func resolveRemoveTarget(cwd, mainRepository, name string, force bool) (*instance.Instance, error) {
	if name != "" {
		instances, err := instance.Discover(mainRepository)
		if err != nil {
			return nil, NewExitError(ExitGenericFailure, err)
		}

		for i := range instances {
			if instances[i].Name == name {
				return &instances[i], nil
			}
		}

		return nil, NewExitError(ExitGenericFailure, fmt.Errorf("no instance named %q", name))
	}

	target, err := compose.ResolveInstance(cwd)
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}
	if target == nil {
		return nil, NewExitError(ExitGenericFailure, fmt.Errorf("no instance name given and cwd is not inside one"))
	}

	if !force {
		if !IsTTY() {
			return nil, NewExitError(
				ExitWouldPrompt,
				fmt.Errorf("refusing to remove %q without --force in non-interactive mode", target.Name),
			)
		}
		if !confirm(fmt.Sprintf("remove instance %q at %s?", target.Name, target.Path)) {
			return nil, nil
		}
	}

	return target, nil
}

func stopInstanceStack(cmd *cobra.Command, target *instance.Instance, keepVolumes bool) {
	runner, err := compose.NewRunner()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: docker unavailable, skipping stack teardown:", err)
		return
	}

	if err := runner.Stop(target, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: stopping stack:", err)
	}
	if err := runner.Down(target, !keepVolumes, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: bringing stack down:", err)
	}
}

// removeWorktree attempts `git worktree remove` without --force first, per
// DESIGN.md: on refusal (a dirty tree), show `git status --short` and
// require the caller's own --force before retrying with one.
func removeWorktree(cmd *cobra.Command, mainRepository string, target *instance.Instance, force bool) error {
	err := gitx.WorktreeRemove(mainRepository, target.Path, false)
	if err == nil {
		return nil
	}

	if force {
		if forceErr := gitx.WorktreeRemove(mainRepository, target.Path, true); forceErr != nil {
			return NewExitError(ExitGenericFailure, forceErr)
		}
		return nil
	}

	status, _ := gitx.StatusShort(target.Path)
	fmt.Fprintln(cmd.ErrOrStderr(), "refusing to remove a dirty worktree:")
	fmt.Fprintln(cmd.ErrOrStderr(), status)

	return NewExitError(
		ExitDirtyTreeRefusal,
		fmt.Errorf("worktree %s has uncommitted changes; rerun with --force to discard them", target.Path),
	)
}

func releaseClaim(target *instance.Instance) error {
	store, err := registry.Open()
	if err != nil {
		return err
	}

	return store.Release(target.Project, target.Name)
}
