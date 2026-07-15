package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/compose"
	"github.com/geocodio/treehouse/internal/registry"
)

func TestDoctorCheckComposeContract_HardcodedPortFails(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
`
	var checks []doctorCheck
	composePath := writeComposeFixture(t, compose)
	doctorCheckComposeContract(composePath, &checks)

	check := findCheck(t, checks, "compose: hardcoded published ports")
	if check.Status != checkFail {
		t.Errorf("status = %v, want FAIL", check.Status)
	}
	if !strings.Contains(check.Detail, "8080:80") {
		t.Errorf("detail = %q, want it to mention the offending port", check.Detail)
	}
}

func TestDoctorCheckComposeContract_ParameterizedPortPasses(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:alpine
    ports:
      - "${HTTP_PORT:-8080}:80"
`
	var checks []doctorCheck
	composePath := writeComposeFixture(t, compose)
	doctorCheckComposeContract(composePath, &checks)

	check := findCheck(t, checks, "compose: hardcoded published ports")
	if check.Status != checkPass {
		t.Errorf("status = %v, want PASS for a parameterized port", check.Status)
	}
}

func TestDoctorCheckComposeContract_ContainerNameFails(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:alpine
    container_name: fixed-name
`
	var checks []doctorCheck
	composePath := writeComposeFixture(t, compose)
	doctorCheckComposeContract(composePath, &checks)

	check := findCheck(t, checks, "compose: container_name")
	if check.Status != checkFail {
		t.Errorf("status = %v, want FAIL", check.Status)
	}
	if !strings.Contains(check.Detail, "fixed-name") {
		t.Errorf("detail = %q, want it to mention the container_name", check.Detail)
	}
}

func TestDoctorCheckComposeContract_AbsoluteBindMountWarns(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:alpine
    volumes:
      - /etc/config:/config
`
	var checks []doctorCheck
	composePath := writeComposeFixture(t, compose)
	doctorCheckComposeContract(composePath, &checks)

	check := findCheck(t, checks, "compose: absolute bind mounts")
	if check.Status != checkWarn {
		t.Errorf("status = %v, want WARN (not FAIL) for an absolute bind mount", check.Status)
	}
}

func TestDoctorCheckComposeContract_RelativeVolumePasses(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:alpine
    volumes:
      - ./web:/usr/share/nginx/html:ro
`
	var checks []doctorCheck
	composePath := writeComposeFixture(t, compose)
	doctorCheckComposeContract(composePath, &checks)

	check := findCheck(t, checks, "compose: absolute bind mounts")
	if check.Status != checkPass {
		t.Errorf("status = %v, want PASS for a relative volume", check.Status)
	}
}

// TestDoctorCheckManifest_ReadsFromSecondaryWorktree guards BUG 1's fix in
// `doctor` specifically (it resolves the manifest itself, independent of
// loadRepositoryContext -- see repo_test.go for the equivalent create/list/
// switch/remove/path coverage): a branch that adds .treehouse.yml, checked
// out into a secondary worktree, must pass doctor's manifest check even
// though the main checkout never touched the manifest.
func TestDoctorCheckManifest_ReadsFromSecondaryWorktree(t *testing.T) {
	main := initRepositoryWithoutManifest(t)

	worktreePath := filepath.Join(t.TempDir(), "feature-add-manifest")
	runGitCommand(t, main, "worktree", "add", "-b", "feature/add-manifest", worktreePath)

	manifestContents := "version: 1\nproject: adopted\n"
	if err := os.WriteFile(filepath.Join(worktreePath, manifestFileName), []byte(manifestContents), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	runGitCommand(t, worktreePath, "add", manifestFileName)
	runGitCommand(t, worktreePath, "commit", "-q", "-m", "add treehouse manifest")

	var checks []doctorCheck
	mainRepository, _ := doctorCheckManifest(worktreePath, &checks)

	if check := findCheck(t, checks, "manifest"); check.Status != checkPass {
		t.Errorf("manifest check status = %v, detail %q, want PASS", check.Status, check.Detail)
	}

	wantMainRepository := evalSymlinksOrFatal(t, main)
	if gotMainRepository := evalSymlinksOrFatal(t, mainRepository); gotMainRepository != wantMainRepository {
		t.Errorf("doctorCheckManifest() mainRepository = %q, want %q", gotMainRepository, wantMainRepository)
	}
}

func TestDoctorCheckRegistry_FlagsStaleClaimAsWarning(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	store, err := registry.Open()
	if err != nil {
		t.Fatalf("registry.Open() returned unexpected error: %v", err)
	}

	staleWorktree := filepath.Join(t.TempDir(), "removed-worktree")
	if _, err := store.Claim(registry.ClaimRequest{
		Project: "myapp", Instance: "stale", Repository: "/repo", Worktree: staleWorktree, BlockSize: 32,
	}); err != nil {
		t.Fatalf("Claim() returned unexpected error: %v", err)
	}

	var checks []doctorCheck
	doctorCheckRegistry(&checks)

	check := findCheck(t, checks, "registry: stale claims")
	if check.Status != checkWarn {
		t.Errorf("status = %v, want WARN", check.Status)
	}
}

func TestDoctorCheckTools_ReportsVersionsWhenToolsPresent(t *testing.T) {
	lookup := func(name string) (string, error) {
		return name + " version 1.2.3", nil
	}
	newRunner := func() (*compose.Runner, error) {
		return &compose.Runner{}, nil
	}

	var checks []doctorCheck
	doctorCheckTools(&checks, lookup, newRunner)

	for _, name := range []string{"git", "docker"} {
		check := findCheck(t, checks, name)
		if check.Status != checkPass {
			t.Errorf("%s check status = %v, detail %q, want PASS", name, check.Status, check.Detail)
		}
		if !strings.Contains(check.Detail, "version 1.2.3") {
			t.Errorf("%s detail = %q, want it to report the tool version", name, check.Detail)
		}
	}

	if check := findCheck(t, checks, "docker compose"); check.Status != checkPass {
		t.Errorf("docker compose check status = %v, want PASS when a runner is available", check.Status)
	}
}

func TestDoctorCheckTools_FailsWithActionableMessageWhenToolsMissing(t *testing.T) {
	lookup := func(name string) (string, error) {
		return "", fmt.Errorf("%s: executable file not found in $PATH", name)
	}
	newRunner := func() (*compose.Runner, error) {
		return nil, compose.ErrDockerUnavailable
	}

	var checks []doctorCheck
	doctorCheckTools(&checks, lookup, newRunner)

	for _, name := range []string{"git", "docker", "docker compose"} {
		check := findCheck(t, checks, name)
		if check.Status != checkFail {
			t.Errorf("%s check status = %v, want FAIL when the tool is missing", name, check.Status)
		}
		if strings.TrimSpace(check.Detail) == "" {
			t.Errorf("%s FAIL check has an empty detail, want an actionable message", name)
		}
	}

	if detail := findCheck(t, checks, "docker").Detail; !strings.Contains(detail, "install Docker") {
		t.Errorf("docker FAIL detail = %q, want it to tell the user how to remediate", detail)
	}
}

// TestDoctorCheckToolVersions_HostIndependentWhenToolsAbsent exercises the
// real production probes against an emptied PATH, approximating a barren
// runner such as GitHub's macOS runners (which have no docker). Every tool
// check must FAIL cleanly rather than pass on host state or hang -- this is
// the regression that turned CI red on macOS.
func TestDoctorCheckToolVersions_HostIndependentWhenToolsAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var checks []doctorCheck
	doctorCheckToolVersions(&checks)

	for _, name := range []string{"git", "docker", "docker compose"} {
		if check := findCheck(t, checks, name); check.Status != checkFail {
			t.Errorf("%s check status = %v, want FAIL when no tools are on PATH", name, check.Status)
		}
	}
}

func findCheck(t *testing.T, checks []doctorCheck, name string) doctorCheck {
	t.Helper()

	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}

	t.Fatalf("no check named %q found among %+v", name, checks)
	return doctorCheck{}
}

func writeComposeFixture(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing compose fixture: %v", err)
	}

	return path
}
