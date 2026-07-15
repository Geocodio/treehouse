package compose

import (
	"reflect"
	"testing"
	"time"

	"github.com/geocodio/treehouse/internal/instance"
)

func TestBuildArgs_MainRepositoryPassesThroughUnchanged(t *testing.T) {
	got := buildArgs([]string{"docker", "compose"}, nil, []string{"ps"})
	want := []string{"compose", "ps"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArgs() = %v, want %v", got, want)
	}
}

func TestBuildArgs_InstanceInjectsEnvFileAndProjectName(t *testing.T) {
	inst := instance.New(instance.NewParams{
		Project: "myapp", Name: "feature-a", Branch: "feature-a",
		MainRepository: "/repo", Path: "/treehouses/myapp/feature-a",
		BlockStart: 4000, Ports: map[string]int{"HTTP_PORT": 4000},
	})

	got := buildArgs([]string{"docker", "compose"}, &inst, []string{"up", "-d"})
	want := []string{
		"compose",
		"--env-file", "/treehouses/myapp/feature-a/.treehouse.env",
		"--project-name", "myapp-feature-a",
		"up", "-d",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArgs() = %v, want %v", got, want)
	}
}

func TestBuildArgs_DockerComposeFallbackHasNoLeadingSubcommand(t *testing.T) {
	got := buildArgs([]string{"docker-compose"}, nil, []string{"ps"})
	want := []string{"ps"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArgs() = %v, want %v", got, want)
	}
}

func TestParseContainerLines(t *testing.T) {
	output := []byte("abc123\tmyapp-feature-a-web-1\trunning\ndef456\tmyapp-feature-a-cache-1\texited\n")

	got := parseContainerLines(output)
	want := []Container{
		{ID: "abc123", Name: "myapp-feature-a-web-1", State: "running"},
		{ID: "def456", Name: "myapp-feature-a-cache-1", State: "exited"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseContainerLines() = %+v, want %+v", got, want)
	}
}

func TestParseContainerLines_Empty(t *testing.T) {
	got := parseContainerLines([]byte(""))
	if len(got) != 0 {
		t.Errorf("parseContainerLines(\"\") = %v, want empty", got)
	}
}

func TestParsePublishedPorts(t *testing.T) {
	// Representative `docker ps --format "{{.Ports}}"` output: a wildcard
	// IPv4 binding, an IPv6 binding for the same container, and a container
	// port with no host mapping at all (must not be parsed as a port).
	output := []byte("0.0.0.0:4640->80/tcp, :::4640->80/tcp\n6379/tcp\n0.0.0.0:4641->6379/tcp\n")

	got := parsePublishedPorts(output)
	want := map[int]bool{4640: true, 4641: true}

	if len(got) != len(want) {
		t.Fatalf("parsePublishedPorts() = %v, want %v", got, want)
	}
	for port := range want {
		if !got[port] {
			t.Errorf("parsePublishedPorts() missing port %d", port)
		}
	}
}

func TestParsePublishedPorts_Empty(t *testing.T) {
	got := parsePublishedPorts([]byte(""))
	if len(got) != 0 {
		t.Errorf("parsePublishedPorts(\"\") = %v, want empty", got)
	}
}

func TestParseContainerActivity_TakesNewestAcrossContainers(t *testing.T) {
	// Container 1 is still running (FinishedAt is docker's zero-time
	// sentinel, which must be ignored); container 2 exited more recently
	// than container 1 started.
	output := []byte(
		"2026-07-10T10:00:00.000000000Z\t0001-01-01T00:00:00Z\n" +
			"2026-07-01T00:00:00.000000000Z\t2026-07-12T09:30:00.000000000Z\n",
	)

	got := parseContainerActivity(output)

	want, err := time.Parse(time.RFC3339Nano, "2026-07-12T09:30:00.000000000Z")
	if err != nil {
		t.Fatalf("parsing want time: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("parseContainerActivity() = %v, want %v", got, want)
	}
}

func TestParseContainerActivity_EmptyOutputIsZeroTime(t *testing.T) {
	got := parseContainerActivity([]byte(""))
	if !got.IsZero() {
		t.Errorf("parseContainerActivity(\"\") = %v, want zero time", got)
	}
}
