package compose

import (
	"reflect"
	"testing"

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
