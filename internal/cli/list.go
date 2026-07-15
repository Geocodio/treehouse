package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/registry"
)

func newListCommand() *cobra.Command {
	var jsonOutput, allRepos bool

	command := &cobra.Command{
		Use:   "list",
		Short: "List instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, jsonOutput, allRepos)
		},
	}

	command.Flags().BoolVar(&jsonOutput, "json", false, "emit a single JSON document on stdout")
	command.Flags().BoolVar(&allRepos, "all-repos", false, "list instances across every treehouse-managed repository")

	return command
}

// listEntry is one instance's row in `list`'s output, both human-readable
// and --json.
type listEntry struct {
	Project           string         `json:"project"`
	Instance          string         `json:"instance"`
	Branch            string         `json:"branch"`
	Path              string         `json:"path"`
	ComposeProject    string         `json:"compose_project"`
	BlockStart        int            `json:"block_start"`
	Ports             map[string]int `json:"ports"`
	State             instance.State `json:"state"`
	RunningContainers int            `json:"running_containers"`
}

func runList(cmd *cobra.Command, jsonOutput, allRepos bool) error {
	var entries []listEntry
	var err error

	if allRepos {
		entries, err = listAllRepos()
	} else {
		entries, err = listCurrentRepo(cmd)
	}
	if err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(cmd, entries)
	}

	printListTable(cmd, entries)
	return nil
}

func listCurrentRepo(cmd *cobra.Command) ([]listEntry, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	repo, err := loadRepositoryContext(cwd)
	if err != nil {
		return nil, err
	}
	printWarnings(cmd, repo.Warnings)

	instances, err := instance.Discover(repo.MainRepository)
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	entries := make([]listEntry, 0, len(instances))
	for _, inst := range instances {
		entries = append(entries, entryFromInstance(inst))
	}

	return entries, nil
}

// listAllRepos reads the machine-global registry directly rather than
// Discover, since --all-repos must cover every treehouse-managed
// repository on the machine, not just the one containing cwd.
func listAllRepos() ([]listEntry, error) {
	store, err := registry.Open()
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	claims, err := store.List()
	if err != nil {
		return nil, NewExitError(ExitGenericFailure, err)
	}

	entries := make([]listEntry, 0, len(claims))
	for _, claim := range claims {
		entry := listEntry{
			Project:    claim.Project,
			Instance:   claim.Instance,
			Path:       claim.Worktree,
			BlockStart: claim.BlockStart,
		}

		envFilePath := filepath.Join(claim.Worktree, instance.EnvFileName)
		if loaded, loadErr := instance.LoadEnvFile(envFilePath); loadErr == nil {
			entry.Branch = loaded.Branch
			entry.ComposeProject = loaded.ComposeProject
			entry.Ports = loaded.Ports
			entry.State = loaded.State
		}

		if entry.ComposeProject != "" {
			if count, countErr := compose.RunningContainerCount(entry.ComposeProject); countErr == nil {
				entry.RunningContainers = count
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func entryFromInstance(inst instance.Instance) listEntry {
	entry := listEntry{
		Project:        inst.Project,
		Instance:       inst.Name,
		Branch:         inst.Branch,
		Path:           inst.Path,
		ComposeProject: inst.ComposeProject,
		BlockStart:     inst.BlockStart,
		Ports:          inst.Ports,
		State:          inst.State,
	}

	if count, err := compose.RunningContainerCount(inst.ComposeProject); err == nil {
		entry.RunningContainers = count
	}

	return entry
}

func printListTable(cmd *cobra.Command, entries []listEntry) {
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no instances found")
		return
	}

	for _, entry := range entries {
		fmt.Fprintf(
			cmd.OutOrStdout(),
			"%s/%s\tbranch=%s\tstate=%s\tcontainers=%d\t%s\n",
			entry.Project, entry.Instance, entry.Branch, entry.State, entry.RunningContainers, entry.Path,
		)
	}
}
