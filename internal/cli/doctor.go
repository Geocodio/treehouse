package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/gitx"
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/manifest"
	"github.com/geocodio/treehouse/internal/registry"
)

// checkStatus is one doctorCheck's outcome.
type checkStatus string

const (
	checkPass checkStatus = "PASS"
	checkWarn checkStatus = "WARN"
	checkFail checkStatus = "FAIL"
)

// doctorCheck is a single pass/warn/fail line in doctor's report.
type doctorCheck struct {
	Name   string
	Status checkStatus
	Detail string
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check this repository's treehouse setup and the local environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd)
		},
	}
}

// runDoctor runs every check independently — a missing or invalid manifest
// doesn't stop the tool/registry checks from running too — then exits
// nonzero if any check failed, per DESIGN.md's "clear pass/warn/fail lines
// and nonzero exit on fail" rule.
func runDoctor(cmd *cobra.Command) error {
	var checks []doctorCheck

	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	mainRepository, composePath := doctorCheckManifest(cwd, &checks)
	if composePath != "" {
		doctorCheckComposeContract(composePath, &checks)
	}

	doctorCheckToolVersions(&checks)
	doctorCheckRegistry(&checks)

	if mainRepository != "" {
		doctorCheckInstances(mainRepository, &checks)
	}

	printDoctorReport(cmd, checks)

	for _, check := range checks {
		if check.Status == checkFail {
			return NewExitError(ExitGenericFailure, fmt.Errorf("doctor found failing checks"))
		}
	}

	return nil
}

// doctorCheckManifest resolves this checkout's own root and its
// repository's main worktree, then validates the manifest read from the
// checkout root, returning the main repository path so later checks
// (instances) can reuse it without re-deriving it. Either return value is
// empty when that step itself failed — callers skip the checks that depend
// on it.
//
// The manifest (and the docker-compose.yml checked by
// doctorCheckComposeContract) are read from the checkout root, not the
// main repository, for the same reason loadRepositoryContext is: a branch
// that adds .treehouse.yml in a secondary worktree must be diagnosable
// from that worktree even when the main checkout doesn't have the file.
func doctorCheckManifest(cwd string, checks *[]doctorCheck) (mainRepository, composePath string) {
	checkoutRoot, err := gitx.WorktreeRoot(cwd)
	if err != nil {
		*checks = append(*checks, doctorCheck{"git repository", checkFail, err.Error()})
		return "", ""
	}

	mainRepo, err := gitx.FindMainRepository(cwd)
	if err != nil {
		*checks = append(*checks, doctorCheck{"git repository", checkFail, err.Error()})
		return "", ""
	}

	manifestPath := filepath.Join(checkoutRoot, manifestFileName)
	if _, statErr := os.Stat(manifestPath); statErr != nil {
		*checks = append(*checks, doctorCheck{"manifest", checkFail, fmt.Sprintf("%s not found", manifestPath)})
		return mainRepo, ""
	}

	_, warnings, err := manifest.Load(manifestPath)
	if err != nil {
		*checks = append(*checks, doctorCheck{"manifest", checkFail, err.Error()})
		return mainRepo, ""
	}

	if len(warnings) > 0 {
		messages := make([]string, len(warnings))
		for i, warning := range warnings {
			messages[i] = warning.String()
		}
		*checks = append(*checks, doctorCheck{"manifest", checkWarn, strings.Join(messages, "; ")})
	} else {
		*checks = append(*checks, doctorCheck{"manifest", checkPass, ""})
	}

	composeCandidate := filepath.Join(checkoutRoot, "docker-compose.yml")
	if _, statErr := os.Stat(composeCandidate); statErr == nil {
		composePath = composeCandidate
	}

	return mainRepo, composePath
}

// doctorCheckComposeContract checks docker-compose.yml against DESIGN.md's
// contract: a hardcoded published port or a container_name are both fatal
// (they block running more than one instance concurrently), an absolute
// bind mount is only a warning (it may still work, just not portably).
func doctorCheckComposeContract(composePath string, checks *[]doctorCheck) {
	data, err := os.ReadFile(composePath)
	if err != nil {
		*checks = append(*checks, doctorCheck{"compose contract", checkFail, err.Error()})
		return
	}

	services, err := parseComposeServices(data)
	if err != nil {
		*checks = append(*checks, doctorCheck{"compose contract", checkFail, err.Error()})
		return
	}

	var hardcodedPorts, containerNames, absoluteBindMounts []string

	for _, service := range services {
		for _, raw := range service.Ports {
			if parsed := parsePortEntry(service.Name, raw); parsed != nil && parsed.HardcodedHost != "" {
				hardcodedPorts = append(hardcodedPorts, fmt.Sprintf("%s: %q", service.Name, raw))
			}
		}

		if service.ContainerName != "" {
			containerNames = append(containerNames, fmt.Sprintf("%s: %q", service.Name, service.ContainerName))
		}

		for _, volume := range service.Volumes {
			hostPath, _, _ := strings.Cut(volume, ":")
			if filepath.IsAbs(hostPath) {
				absoluteBindMounts = append(absoluteBindMounts, fmt.Sprintf("%s: %q", service.Name, volume))
			}
		}
	}

	appendComposeCheck(checks, "compose: hardcoded published ports", checkFail, hardcodedPorts, "use ${VAR:-default}; see `treehouse init`")
	appendComposeCheck(checks, "compose: container_name", checkFail, containerNames, "prevents multiple instances running concurrently")
	appendComposeCheck(checks, "compose: absolute bind mounts", checkWarn, absoluteBindMounts, "")
}

func appendComposeCheck(checks *[]doctorCheck, name string, failStatus checkStatus, offenders []string, hint string) {
	if len(offenders) == 0 {
		*checks = append(*checks, doctorCheck{name, checkPass, ""})
		return
	}

	detail := strings.Join(offenders, "; ")
	if hint != "" {
		detail += " (" + hint + ")"
	}
	*checks = append(*checks, doctorCheck{name, failStatus, detail})
}

func doctorCheckToolVersions(checks *[]doctorCheck) {
	if output, err := exec.Command("git", "--version").Output(); err == nil {
		*checks = append(*checks, doctorCheck{"git", checkPass, strings.TrimSpace(string(output))})
	} else {
		*checks = append(*checks, doctorCheck{"git", checkFail, "git not found or not runnable"})
	}

	if output, err := exec.Command("docker", "--version").Output(); err == nil {
		*checks = append(*checks, doctorCheck{"docker", checkPass, strings.TrimSpace(string(output))})
	} else {
		*checks = append(*checks, doctorCheck{"docker", checkFail, "docker not found or not runnable"})
	}

	if _, err := compose.NewRunner(); err == nil {
		*checks = append(*checks, doctorCheck{"docker compose", checkPass, ""})
	} else {
		*checks = append(*checks, doctorCheck{"docker compose", checkFail, "neither `docker compose` nor `docker-compose` is available"})
	}
}

func doctorCheckRegistry(checks *[]doctorCheck) {
	store, err := registry.Open()
	if err != nil {
		*checks = append(*checks, doctorCheck{"registry", checkFail, err.Error()})
		return
	}

	claims, err := store.List()
	if err != nil {
		*checks = append(*checks, doctorCheck{"registry", checkFail, err.Error()})
		return
	}

	var stale []string
	for _, claim := range claims {
		if _, statErr := os.Stat(claim.Worktree); statErr != nil {
			stale = append(stale, fmt.Sprintf("%s/%s", claim.Project, claim.Instance))
		}
	}

	if len(stale) == 0 {
		*checks = append(*checks, doctorCheck{"registry: stale claims", checkPass, ""})
		return
	}

	*checks = append(*checks, doctorCheck{
		"registry: stale claims", checkWarn,
		fmt.Sprintf("%s (run `treehouse gc` to reap)", strings.Join(stale, ", ")),
	})
}

// doctorCheckInstances walks git worktrees directly (rather than
// instance.Discover, which fails its whole call on the first unparseable
// .treehouse.env) so one bad instance is reported as its own failing check
// instead of hiding every other check behind it.
func doctorCheckInstances(mainRepository string, checks *[]doctorCheck) {
	entries, err := gitx.ListWorktrees(mainRepository)
	if err != nil {
		*checks = append(*checks, doctorCheck{"instances", checkFail, err.Error()})
		return
	}

	var claims []registry.Claim
	if store, err := registry.Open(); err == nil {
		claims, _ = store.List()
	}

	hasClaim := func(project, name string) bool {
		for _, claim := range claims {
			if claim.Project == project && claim.Instance == name {
				return true
			}
		}
		return false
	}

	checked := 0
	for _, entry := range entries {
		envFilePath := filepath.Join(entry.Path, instance.EnvFileName)
		if _, statErr := os.Stat(envFilePath); statErr != nil {
			continue
		}
		checked++

		loaded, loadErr := instance.LoadEnvFile(envFilePath)
		if loadErr != nil {
			*checks = append(*checks, doctorCheck{
				fmt.Sprintf("instance env: %s", entry.Path), checkFail, loadErr.Error(),
			})
			continue
		}

		if !hasClaim(loaded.Project, loaded.Name) {
			*checks = append(*checks, doctorCheck{
				fmt.Sprintf("instance claim: %s/%s", loaded.Project, loaded.Name), checkWarn,
				"no matching registry claim (orphaned .treehouse.env)",
			})
		}
	}

	if checked == 0 {
		*checks = append(*checks, doctorCheck{"instances", checkPass, "none found"})
	}
}

func printDoctorReport(cmd *cobra.Command, checks []doctorCheck) {
	out := cmd.OutOrStdout()
	for _, check := range checks {
		line := fmt.Sprintf("[%s] %s", check.Status, check.Name)
		if check.Detail != "" {
			line += ": " + check.Detail
		}
		fmt.Fprintln(out, line)
	}
}
