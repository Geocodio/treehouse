package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParsePortEntry_ExistingSubstitutionIsAdoptedAsIs(t *testing.T) {
	got := parsePortEntry("web", "${HTTP_PORT:-8080}:80")
	if got == nil {
		t.Fatalf("parsePortEntry() = nil, want a parsed entry")
	}
	if got.ExistingVar != "HTTP_PORT" {
		t.Errorf("ExistingVar = %q, want HTTP_PORT", got.ExistingVar)
	}
	if got.ExistingDefault != "8080" {
		t.Errorf("ExistingDefault = %q, want 8080", got.ExistingDefault)
	}
	if got.HardcodedHost != "" {
		t.Errorf("HardcodedHost = %q, want empty", got.HardcodedHost)
	}
}

func TestParsePortEntry_BareSubstitutionWithNoDefault(t *testing.T) {
	got := parsePortEntry("web", "${HTTP_PORT}:80")
	if got == nil {
		t.Fatalf("parsePortEntry() = nil, want a parsed entry")
	}
	if got.ExistingVar != "HTTP_PORT" {
		t.Errorf("ExistingVar = %q, want HTTP_PORT", got.ExistingVar)
	}
}

func TestParsePortEntry_HardcodedHostPort(t *testing.T) {
	got := parsePortEntry("web", "8080:80")
	if got == nil {
		t.Fatalf("parsePortEntry() = nil, want a parsed entry")
	}
	if got.HardcodedHost != "8080" {
		t.Errorf("HardcodedHost = %q, want 8080", got.HardcodedHost)
	}
	if got.ExistingVar != "" {
		t.Errorf("ExistingVar = %q, want empty", got.ExistingVar)
	}
}

func TestParsePortEntry_HardcodedWithProtocolSuffix(t *testing.T) {
	got := parsePortEntry("cache", "6379:6379/tcp")
	if got == nil {
		t.Fatalf("parsePortEntry() = nil, want a parsed entry")
	}
	if got.HardcodedHost != "6379" {
		t.Errorf("HardcodedHost = %q, want 6379", got.HardcodedHost)
	}
}

func TestParsePortEntry_HardcodedWithBindIP(t *testing.T) {
	got := parsePortEntry("web", "127.0.0.1:8080:80")
	if got == nil {
		t.Fatalf("parsePortEntry() = nil, want a parsed entry")
	}
	if got.HardcodedHost != "8080" {
		t.Errorf("HardcodedHost = %q, want 8080 (bind IP ignored)", got.HardcodedHost)
	}
}

func TestParsePortEntry_BareContainerPortIsSkipped(t *testing.T) {
	if got := parsePortEntry("web", "80"); got != nil {
		t.Errorf("parsePortEntry() = %+v, want nil for a bare container port", got)
	}
}

func TestDeriveVarName_DerivesFromServiceName(t *testing.T) {
	used := map[string]bool{}
	if got := deriveVarName("web", used); got != "WEB_PORT" {
		t.Errorf("deriveVarName(web) = %q, want WEB_PORT", got)
	}
	if got := deriveVarName("redis", used); got != "REDIS_PORT" {
		t.Errorf("deriveVarName(redis) = %q, want REDIS_PORT", got)
	}
}

func TestDeriveVarName_AppendsSuffixOnCollision(t *testing.T) {
	used := map[string]bool{}
	first := deriveVarName("web", used)
	second := deriveVarName("web", used)

	if first != "WEB_PORT" {
		t.Errorf("first = %q, want WEB_PORT", first)
	}
	if second != "WEB_PORT_2" {
		t.Errorf("second = %q, want WEB_PORT_2", second)
	}
}

func TestProposeManifestPorts_MixesAdoptedAndProposedNames(t *testing.T) {
	ports := []publishedPort{
		{ServiceName: "web", Raw: "${HTTP_PORT:-8080}:80", ExistingVar: "HTTP_PORT", ExistingDefault: "8080"},
		{ServiceName: "cache", Raw: "6379:6379", HardcodedHost: "6379"},
	}

	envNames, diffs := proposeManifestPorts(ports)

	want := []string{"HTTP_PORT", "CACHE_PORT"}
	if !reflect.DeepEqual(envNames, want) {
		t.Errorf("envNames = %v, want %v", envNames, want)
	}

	if len(diffs) != 1 {
		t.Fatalf("len(diffs) = %d, want 1", len(diffs))
	}
	if !strings.Contains(diffs[0], "6379:6379") || !strings.Contains(diffs[0], "${CACHE_PORT:-6379}:6379") {
		t.Errorf("diffs[0] = %q, want it to show the before/after port mapping", diffs[0])
	}
}

func TestRenderManifest_OrdersEnvEntriesByIndex(t *testing.T) {
	got := renderManifest("myapp", []string{"HTTP_PORT", "CACHE_PORT"})

	if !strings.Contains(got, "project: myapp") {
		t.Errorf("renderManifest() = %q, missing project line", got)
	}
	if !strings.Contains(got, "    HTTP_PORT: 0") || !strings.Contains(got, "    CACHE_PORT: 1") {
		t.Errorf("renderManifest() = %q, missing expected ports.env entries", got)
	}
	if strings.Index(got, "HTTP_PORT") > strings.Index(got, "CACHE_PORT") {
		t.Errorf("renderManifest() ordered CACHE_PORT before HTTP_PORT, want discovery order preserved")
	}
}

func TestParseComposeServices_PreservesOrderAndFields(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:alpine
    container_name: fixed-name
    ports:
      - "8080:80"
      - "${HTTP_PORT:-8081}:81"
    volumes:
      - /etc/config:/config
      - ./relative:/data
  cache:
    image: redis:alpine
    ports:
      - "6379:6379"
`
	services, err := parseComposeServices([]byte(compose))
	if err != nil {
		t.Fatalf("parseComposeServices() returned unexpected error: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("len(services) = %d, want 2", len(services))
	}

	if services[0].Name != "web" {
		t.Errorf("services[0].Name = %q, want web (discovery order)", services[0].Name)
	}
	if services[0].ContainerName != "fixed-name" {
		t.Errorf("services[0].ContainerName = %q, want fixed-name", services[0].ContainerName)
	}
	if len(services[0].Ports) != 2 {
		t.Fatalf("len(services[0].Ports) = %d, want 2", len(services[0].Ports))
	}
	if len(services[0].Volumes) != 2 || services[0].Volumes[0] != "/etc/config:/config" {
		t.Errorf("services[0].Volumes = %v, want [/etc/config:/config ./relative:/data]", services[0].Volumes)
	}

	if services[1].Name != "cache" {
		t.Errorf("services[1].Name = %q, want cache", services[1].Name)
	}
}

func TestRunInit_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	composeContents := `
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
  cache:
    image: redis:alpine
    ports:
      - "${CACHE_PORT:-6379}:6379"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(composeContents), 0o644); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(): %v", err)
	}
	defer os.Chdir(original)

	var output bytes.Buffer
	command := NewRootCommand("test")
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"init", "--write"})
	if err := command.Execute(); err != nil {
		t.Fatalf("init --write failed: %v\n%s", err, output.String())
	}

	manifestPath := filepath.Join(dir, manifestFileName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading written manifest: %v", err)
	}
	if !strings.Contains(string(data), "WEB_PORT: 0") {
		t.Errorf("written manifest = %q, missing WEB_PORT: 0", string(data))
	}
	if !strings.Contains(string(data), "CACHE_PORT: 1") {
		t.Errorf("written manifest = %q, missing CACHE_PORT: 1", string(data))
	}

	// --write must refuse to overwrite an existing manifest.
	var secondOutput bytes.Buffer
	secondCommand := NewRootCommand("test")
	secondCommand.SetOut(&secondOutput)
	secondCommand.SetErr(&secondOutput)
	secondCommand.SetArgs([]string{"init", "--write"})
	if err := secondCommand.Execute(); err == nil {
		t.Errorf("second init --write succeeded, want refusal to overwrite an existing manifest")
	}
}
