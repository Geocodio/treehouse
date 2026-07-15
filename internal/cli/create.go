package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/gitx"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/machineconfig"
	"github.com/geocodio/treehouse/internal/manifest"
	"github.com/geocodio/treehouse/internal/registry"
)

func newCreateCommand() *cobra.Command {
	var name string
	var noStart, noProvision, jsonOutput, noGC bool

	command := &cobra.Command{
		Use:   "create <branch>",
		Short: "Create (or reuse) an instance for a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, args[0], name, noStart, noProvision, jsonOutput, noGC)
		},
	}

	command.Flags().StringVar(&name, "name", "", "instance name (default: derived from the branch name)")
	command.Flags().BoolVar(&noStart, "no-start", false, "don't bring the compose stack up")
	command.Flags().BoolVar(&noProvision, "no-provision", false, "skip symlink/copy/hook provisioning")
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit a single JSON document on stdout")
	command.Flags().BoolVar(&noGC, "no-gc", false, "skip the opportunistic gc pass this command would otherwise trigger")

	return command
}

// createResult is create's --json output shape (also reused by the
// idempotent "existing instance" path, with Existing set to true).
type createResult struct {
	Path           string                 `json:"path"`
	Branch         string                 `json:"branch"`
	Instance       string                 `json:"instance"`
	ComposeProject string                 `json:"compose_project"`
	BlockStart     int                    `json:"block_start"`
	Ports          map[string]int         `json:"ports"`
	URLs           []instance.RenderedURL `json:"urls"`
	State          instance.State         `json:"state"`
	Existing       bool                   `json:"existing"`
}

func runCreate(cmd *cobra.Command, branch, explicitName string, noStart, noProvision, jsonOutput, noGC bool) error {
	// Runs before anything else in this command (not after, e.g., claiming
	// a port block), so its rate-limit check reflects genuine prior gc
	// history rather than the side effect of this very command's own
	// registry.Store.Claim call, which also bumps last_gc_at.
	maybeRunOpportunisticGC(cmd, noGC)

	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	repo, err := loadRepositoryContext(cwd)
	if err != nil {
		return err
	}
	printWarnings(cmd, repo.Warnings)

	existingInstances, err := instance.Discover(repo.MainRepository)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	if existing, ok := findByBranch(existingInstances, branch); ok {
		return finishExistingInstance(cmd, repo, existing, jsonOutput)
	}

	return runFreshCreate(cmd, repo, existingInstances, branch, explicitName, noStart, noProvision, jsonOutput)
}

// finishExistingInstance implements create's idempotent-per-branch
// contract: an instance already tracking branch is left as-is, brought up
// if its stack is currently stopped, and reported with existing=true.
func finishExistingInstance(cmd *cobra.Command, repo *repositoryContext, existing instance.Instance, jsonOutput bool) error {
	count, err := compose.RunningContainerCount(existing.ComposeProject)
	if err == nil && count == 0 {
		if err := ensureStackUp(cmd, &existing, repo.Manifest); err != nil {
			return err
		}
	}

	return printCreateResult(cmd, repo.Manifest, &existing, true, jsonOutput)
}

func runFreshCreate(
	cmd *cobra.Command,
	repo *repositoryContext,
	existingInstances []instance.Instance,
	branch, explicitName string,
	noStart, noProvision, jsonOutput bool,
) error {
	name, err := resolveCreateName(cmd, existingInstances, branch, explicitName)
	if err != nil {
		return err
	}

	config, err := machineconfig.Load(registry.DefaultPoolStart, registry.DefaultPoolEnd)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	instancesHome := resolveInstancesHome(repo.Manifest, config)
	instancePath := filepath.Join(instancesHome, repo.Manifest.Project, name)

	// Claim the port block before mutating git or the filesystem at all:
	// per DESIGN.md, a port-claim failure must release nothing, because
	// nothing else has happened yet.
	store, err := registry.Open()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}
	store.PoolStart = config.PoolStart
	store.PoolEnd = config.PoolEnd

	claimResult, err := store.Claim(registry.ClaimRequest{
		Project:    repo.Manifest.Project,
		Instance:   name,
		Repository: repo.MainRepository,
		Worktree:   instancePath,
		BlockSize:  repo.Manifest.Ports.BlockSize,
	})
	if err != nil {
		if err == registry.ErrPoolExhausted {
			return NewExitError(ExitPortPoolExhausted, err)
		}
		return NewExitError(ExitGenericFailure, err)
	}
	for _, warning := range claimResult.Warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warning)
	}

	if err := ensureBranch(repo.MainRepository, branch); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	if err := gitx.WorktreeAdd(repo.MainRepository, instancePath, branch, false); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	ports := instance.PortsForBlock(claimResult.Claim.BlockStart, repo.Manifest.Ports.Env, repo.Manifest.Ports.Aliases)
	inst := instance.New(instance.NewParams{
		Project:        repo.Manifest.Project,
		Name:           name,
		Branch:         branch,
		MainRepository: repo.MainRepository,
		Path:           instancePath,
		BlockStart:     claimResult.Claim.BlockStart,
		Ports:          ports,
	})

	if err := inst.Save(); err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	// Per DESIGN.md's state machine (worktree-created -> deps-cloned ->
	// stack-up -> provisioned), the hook only ever runs once the compose
	// stack is up, since real-world hooks run database migrations that need
	// database containers running. --no-start therefore stops at
	// deps-cloned and skips the hook entirely; `treehouse provision`
	// completes it later.
	if !noProvision {
		if err := materializeDeps(&inst, repo.Manifest); err != nil {
			return err
		}
	}

	if !noStart {
		if err := ensureStackUp(cmd, &inst, repo.Manifest); err != nil {
			return err
		}

		if !noProvision {
			if err := reconcileAndHook(&inst, repo.Manifest); err != nil {
				return err
			}
		}
	}

	return printCreateResult(cmd, repo.Manifest, &inst, false, jsonOutput)
}

// resolveCreateName picks the instance name: an explicit --name is
// validated against DESIGN.md's collision rule (a hard error, unlike the
// auto-suffix that applies to a derived slug); otherwise the name is
// derived from branch via instance.ResolveName, which never fails, only
// warns.
func resolveCreateName(cmd *cobra.Command, existingInstances []instance.Instance, branch, explicitName string) (string, error) {
	if explicitName == "" {
		name, warning := instance.ResolveName(existingInstances, branch)
		if warning != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warning)
		}
		return name, nil
	}

	if collidesWithDifferentBranch(existingInstances, explicitName, branch) {
		return "", NewExitError(
			ExitGenericFailure,
			fmt.Errorf("instance name %q is already used by another branch", explicitName),
		)
	}

	return explicitName, nil
}

// ensureBranch creates branch if it doesn't already exist locally, per
// create's "Branch doesn't exist locally: create it from the current
// branch (tracking branch if it exists on origin)" rule.
func ensureBranch(mainRepository, branch string) error {
	exists, err := gitx.BranchExistsLocal(mainRepository, branch)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	return gitx.CreateBranch(mainRepository, branch)
}

func printCreateResult(cmd *cobra.Command, manifestConfig *manifest.Manifest, inst *instance.Instance, existing, jsonOutput bool) error {
	result := createResult{
		Path:           inst.Path,
		Branch:         inst.Branch,
		Instance:       inst.Name,
		ComposeProject: inst.ComposeProject,
		BlockStart:     inst.BlockStart,
		Ports:          inst.Ports,
		URLs:           instance.RenderURLs(manifestConfig.URLs, inst.Ports),
		State:          inst.State,
		Existing:       existing,
	}

	if jsonOutput {
		return printJSON(cmd, result)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "instance %q at %s (branch %s), state=%s\n", inst.Name, inst.Path, inst.Branch, inst.State)
	for _, url := range result.URLs {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", url.Name, url.URL)
	}

	return nil
}
