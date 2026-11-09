package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
)

const urlsTestManifest = `version: 1
project: myapp

ports:
  env:
    HTTP_PORT: 0
    HTTPS_PORT: 1
    MAIL_UI_PORT: 2

urls:
  - name: App
    url: "https://app.myapp.dev:{HTTPS_PORT}"
  - name: Mail UI
    url: "http://localhost:{MAIL_UI_PORT}/mail"
`

// setupURLsInstance writes a manifest and a saved .treehouse.env into a
// fresh worktree directory, which is all `urls` needs to resolve an
// instance from cwd.
func setupURLsInstance(t *testing.T, manifestContents string) string {
	t.Helper()

	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, manifestFileName), []byte(manifestContents), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}

	inst := instance.New(instance.NewParams{
		Project: "myapp", Name: "feature-a", Branch: "feature/a",
		MainRepository: "/repo", Path: worktree, BlockStart: 4640,
		Ports: map[string]int{"HTTP_PORT": 4640, "HTTPS_PORT": 4641, "MAIL_UI_PORT": 4642},
	})
	if err := inst.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	return worktree
}

func executeURLs(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	root := NewRootCommand("test")
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"urls"}, args...))

	err = root.Execute()

	return out.String(), errOut.String(), err
}

func TestURLs_RendersManifestURLsForCwdInstance(t *testing.T) {
	worktree := setupURLsInstance(t, urlsTestManifest)
	t.Chdir(worktree)

	stdout, _, err := executeURLs(t)
	if err != nil {
		t.Fatalf("urls returned unexpected error: %v", err)
	}

	want := "App      https://app.myapp.dev:4641\nMail UI  http://localhost:4642/mail\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestURLs_ResolvesFromSubdirectory(t *testing.T) {
	worktree := setupURLsInstance(t, urlsTestManifest)
	subdirectory := filepath.Join(worktree, "app", "src")
	if err := os.MkdirAll(subdirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Chdir(subdirectory)

	stdout, _, err := executeURLs(t)
	if err != nil {
		t.Fatalf("urls returned unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "http://localhost:4642/mail") {
		t.Errorf("stdout = %q, want it to contain the rendered Mail UI URL", stdout)
	}
}

func TestURLs_JSONOutput(t *testing.T) {
	worktree := setupURLsInstance(t, urlsTestManifest)
	t.Chdir(worktree)

	stdout, _, err := executeURLs(t, "--json")
	if err != nil {
		t.Fatalf("urls --json returned unexpected error: %v", err)
	}

	var result urlsResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, stdout)
	}
	if result.Instance != "feature-a" {
		t.Errorf("Instance = %q, want feature-a", result.Instance)
	}
	if len(result.URLs) != 2 || result.URLs[0].URL != "https://app.myapp.dev:4641" {
		t.Errorf("URLs = %+v, want the two rendered manifest URLs", result.URLs)
	}
}

func TestURLs_NoURLsConfiguredLeavesStdoutEmpty(t *testing.T) {
	worktree := setupURLsInstance(t, "version: 1\nproject: myapp\nports:\n  env:\n    HTTP_PORT: 0\n")
	t.Chdir(worktree)

	stdout, stderr, err := executeURLs(t)
	if err != nil {
		t.Fatalf("urls returned unexpected error: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "no urls configured") {
		t.Errorf("stderr = %q, want a no-urls note", stderr)
	}
}

func TestURLs_OutsideInstanceWithoutNameIsAnError(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, _, err := executeURLs(t); err == nil {
		t.Errorf("urls outside an instance with no name returned nil error")
	}
}
