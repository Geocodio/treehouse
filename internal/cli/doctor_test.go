package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestDoctorCheckToolVersions_GitAndDockerAreAvailable(t *testing.T) {
	// This environment has both git and docker installed (a prerequisite
	// for the rest of this test suite too), so both must pass.
	var checks []doctorCheck
	doctorCheckToolVersions(&checks)

	if findCheck(t, checks, "git").Status != checkPass {
		t.Errorf("git check did not pass in an environment with git installed")
	}
	if findCheck(t, checks, "docker").Status != checkPass {
		t.Errorf("docker check did not pass in an environment with docker installed")
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
