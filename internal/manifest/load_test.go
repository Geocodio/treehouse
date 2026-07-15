package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest writes contents to a .treehouse.yml inside a fresh temp dir
// and returns its path.
func writeManifest(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, ".treehouse.yml")

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing fixture manifest: %v", err)
	}

	return path
}

func TestLoad_ValidMinimalManifest(t *testing.T) {
	path := writeManifest(t, `
version: 1
project: myapp
`)

	result, warnings, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Load() returned unexpected warnings: %v", warnings)
	}

	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}
	if result.Project != "myapp" {
		t.Errorf("Project = %q, want %q", result.Project, "myapp")
	}
	if result.Ports.BlockSize != DefaultBlockSize {
		t.Errorf("Ports.BlockSize = %d, want default %d", result.Ports.BlockSize, DefaultBlockSize)
	}
	if len(result.Ports.Env) != 0 {
		t.Errorf("Ports.Env = %v, want empty", result.Ports.Env)
	}
}

func TestLoad_FullManifestFromDesignDoc(t *testing.T) {
	path := writeManifest(t, `
version: 1
project: myapp

ports:
  block_size: 32
  env:
    HTTP_PORT: 0
    DATABASE_PORT: 1
    MAIL_UI_PORT: 2

provision:
  symlink:
    - data/shared
  copy:
    - path: vendor
      reconcile:
        lockfile: composer.lock
        command: composer install
    - path: node_modules
      reconcile:
        lockfile: package-lock.json
        command: npm install
    - public/build
  copy_files:
    - .env.docker
  hook: scripts/treehouse-post-create.sh

urls:
  - name: App
    url: "http://localhost:{HTTP_PORT}"

location: ~/treehouses
`)

	result, warnings, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Load() returned unexpected warnings: %v", warnings)
	}

	if result.Ports.BlockSize != 32 {
		t.Errorf("Ports.BlockSize = %d, want 32", result.Ports.BlockSize)
	}
	wantEnv := map[string]int{"HTTP_PORT": 0, "DATABASE_PORT": 1, "MAIL_UI_PORT": 2}
	for name, index := range wantEnv {
		if result.Ports.Env[name] != index {
			t.Errorf("Ports.Env[%q] = %d, want %d", name, result.Ports.Env[name], index)
		}
	}

	if len(result.Provision.Symlink) != 1 || result.Provision.Symlink[0] != "data/shared" {
		t.Errorf("Provision.Symlink = %v, want [data/shared]", result.Provision.Symlink)
	}

	if len(result.Provision.Copy) != 3 {
		t.Fatalf("len(Provision.Copy) = %d, want 3", len(result.Provision.Copy))
	}
	if result.Provision.Copy[0].Path != "vendor" {
		t.Errorf("Copy[0].Path = %q, want vendor", result.Provision.Copy[0].Path)
	}
	if result.Provision.Copy[0].Reconcile == nil {
		t.Fatalf("Copy[0].Reconcile = nil, want set")
	}
	if result.Provision.Copy[0].Reconcile.Lockfile != "composer.lock" {
		t.Errorf("Copy[0].Reconcile.Lockfile = %q, want composer.lock", result.Provision.Copy[0].Reconcile.Lockfile)
	}
	if result.Provision.Copy[0].Reconcile.Command != "composer install" {
		t.Errorf("Copy[0].Reconcile.Command = %q, want composer install", result.Provision.Copy[0].Reconcile.Command)
	}
	if result.Provision.Copy[2].Path != "public/build" {
		t.Errorf("Copy[2].Path = %q, want public/build", result.Provision.Copy[2].Path)
	}
	if result.Provision.Copy[2].Reconcile != nil {
		t.Errorf("Copy[2].Reconcile = %v, want nil (plain string form)", result.Provision.Copy[2].Reconcile)
	}

	if len(result.Provision.CopyFiles) != 1 || result.Provision.CopyFiles[0] != ".env.docker" {
		t.Errorf("Provision.CopyFiles = %v, want [.env.docker]", result.Provision.CopyFiles)
	}
	if result.Provision.Hook != "scripts/treehouse-post-create.sh" {
		t.Errorf("Provision.Hook = %q, want scripts/treehouse-post-create.sh", result.Provision.Hook)
	}

	if len(result.URLs) != 1 || result.URLs[0].Name != "App" || result.URLs[0].URL != "http://localhost:{HTTP_PORT}" {
		t.Errorf("URLs = %+v, want single App entry", result.URLs)
	}

	if result.Location != "~/treehouses" {
		t.Errorf("Location = %q, want ~/treehouses", result.Location)
	}
}

func TestLoad_PortsAliases(t *testing.T) {
	path := writeManifest(t, `
version: 1
project: myapp
ports:
  env:
    HTTP_PORT: 0
  aliases:
    WEB_PORT: HTTP_PORT
`)

	result, warnings, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Load() returned unexpected warnings: %v", warnings)
	}

	if result.Ports.Aliases["WEB_PORT"] != "HTTP_PORT" {
		t.Errorf("Ports.Aliases[WEB_PORT] = %q, want HTTP_PORT", result.Ports.Aliases["WEB_PORT"])
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantContain string
	}{
		{
			name:        "missing version",
			yaml:        "project: myapp\n",
			wantContain: "version: is required",
		},
		{
			name:        "missing project",
			yaml:        "version: 1\n",
			wantContain: "project: is required",
		},
		{
			name:        "version too new",
			yaml:        "version: 2\nproject: myapp\n",
			wantContain: "newer than the highest version",
		},
		{
			name:        "version zero",
			yaml:        "version: 0\nproject: myapp\n",
			wantContain: "version: must be at least 1",
		},
		{
			name:        "project uppercase",
			yaml:        "version: 1\nproject: MyApp\n",
			wantContain: "project \"MyApp\" must match",
		},
		{
			name:        "project leading hyphen",
			yaml:        "version: 1\nproject: -myapp\n",
			wantContain: "project \"-myapp\" must match",
		},
		{
			name: "duplicate port indices",
			yaml: `
version: 1
project: myapp
ports:
  env:
    HTTP_PORT: 0
    OTHER_PORT: 0
`,
			wantContain: "is already used by ports.env.HTTP_PORT",
		},
		{
			name: "port index exceeds block size",
			yaml: `
version: 1
project: myapp
ports:
  block_size: 4
  env:
    HTTP_PORT: 10
`,
			wantContain: "must be less than ports.block_size (4)",
		},
		{
			name: "negative port index",
			yaml: `
version: 1
project: myapp
ports:
  env:
    HTTP_PORT: -1
`,
			wantContain: "must not be negative",
		},
		{
			name: "invalid env var name",
			yaml: `
version: 1
project: myapp
ports:
  env:
    1PORT: 0
`,
			wantContain: "is not a valid environment variable name",
		},
		{
			name: "alias canonical not defined in ports.env",
			yaml: `
version: 1
project: myapp
ports:
  env:
    HTTP_PORT: 0
  aliases:
    WEB_PORT: MISSING_PORT
`,
			wantContain: `ports.aliases.WEB_PORT: canonical variable "MISSING_PORT" is not defined in ports.env`,
		},
		{
			name: "alias collides with ports.env key",
			yaml: `
version: 1
project: myapp
ports:
  env:
    HTTP_PORT: 0
  aliases:
    HTTP_PORT: HTTP_PORT
`,
			wantContain: "ports.aliases.HTTP_PORT: collides with ports.env.HTTP_PORT",
		},
		{
			name: "symlink absolute path",
			yaml: `
version: 1
project: myapp
provision:
  symlink:
    - /etc/passwd
`,
			wantContain: "must be relative",
		},
		{
			name: "copy_files escapes repository root",
			yaml: `
version: 1
project: myapp
provision:
  copy_files:
    - ../secrets.env
`,
			wantContain: "escapes the repository root",
		},
		{
			name: "hook escapes repository root",
			yaml: `
version: 1
project: myapp
provision:
  hook: ../../evil.sh
`,
			wantContain: "escapes the repository root",
		},
		{
			name: "copy entry missing path",
			yaml: `
version: 1
project: myapp
provision:
  copy:
    - reconcile:
        lockfile: composer.lock
        command: composer install
`,
			wantContain: "missing required key \"path\"",
		},
		{
			name: "reconcile missing command",
			yaml: `
version: 1
project: myapp
provision:
  copy:
    - path: vendor
      reconcile:
        lockfile: composer.lock
`,
			wantContain: "requires both \"lockfile\" and \"command\"",
		},
		{
			name: "unknown top-level key",
			yaml: `
version: 1
project: myapp
banana: true
`,
			wantContain: `unknown top-level key "banana"`,
		},
		{
			name: "urls entry missing url",
			yaml: `
version: 1
project: myapp
urls:
  - name: App
`,
			wantContain: `urls[0]: is missing required key "url"`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			path := writeManifest(t, testCase.yaml)

			_, _, err := Load(path)
			if err == nil {
				t.Fatalf("Load() returned no error, want error containing %q", testCase.wantContain)
			}
			if !strings.Contains(err.Error(), testCase.wantContain) {
				t.Errorf("Load() error = %q, want to contain %q", err.Error(), testCase.wantContain)
			}
		})
	}
}

func TestLoad_UnknownNestedKeysWarnRatherThanFail(t *testing.T) {
	path := writeManifest(t, `
version: 1
project: myapp
ports:
  block_size: 32
  bloc_size: 32
provision:
  copy:
    - path: vendor
      extra: true
`)

	result, warnings, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatalf("Load() returned nil manifest despite only warnings")
	}

	if len(warnings) != 2 {
		t.Fatalf("len(warnings) = %d, want 2: %+v", len(warnings), warnings)
	}

	joined := make([]string, len(warnings))
	for i, w := range warnings {
		joined[i] = w.String()
	}
	all := strings.Join(joined, "\n")

	if !strings.Contains(all, "ports.bloc_size") {
		t.Errorf("warnings = %v, want one mentioning ports.bloc_size", joined)
	}
	if !strings.Contains(all, "provision.copy.extra") {
		t.Errorf("warnings = %v, want one mentioning provision.copy.extra", joined)
	}
}

func TestLoad_MultipleErrorsAggregated(t *testing.T) {
	path := writeManifest(t, `
project: MyApp
`)

	_, _, err := Load(path)
	if err == nil {
		t.Fatalf("Load() returned no error")
	}

	if !strings.Contains(err.Error(), "version: is required") {
		t.Errorf("error missing version problem: %v", err)
	}
	if !strings.Contains(err.Error(), "project \"MyApp\" must match") {
		t.Errorf("error missing project problem: %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err == nil {
		t.Fatalf("Load() returned no error for missing file")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	path := writeManifest(t, "")

	_, _, err := Load(path)
	if err == nil {
		t.Fatalf("Load() returned no error for empty file")
	}
}

func TestLoad_TopLevelNotAMapping(t *testing.T) {
	path := writeManifest(t, "- 1\n- 2\n")

	_, _, err := Load(path)
	if err == nil {
		t.Fatalf("Load() returned no error for non-mapping top level")
	}
	if !strings.Contains(err.Error(), "must be a mapping") {
		t.Errorf("error = %v, want to mention 'must be a mapping'", err)
	}
}
