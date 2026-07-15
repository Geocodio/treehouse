package instance

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnvFile_RoundTrip(t *testing.T) {
	original := Instance{
		Name:           "my-feature",
		Branch:         "feature/my-feature",
		Project:        "myapp",
		MainRepository: "/home/user/projects/myapp",
		Path:           t.TempDir(),
		ComposeProject: "myapp-my-feature",
		BlockStart:     4640,
		State:          StateProvisioned,
		LockHashes: map[string]string{
			"composer.lock":     "aaaa",
			"package-lock.json": "bbbb",
		},
		Ports: map[string]int{
			"HTTP_PORT":     4640,
			"DATABASE_PORT": 4641,
		},
	}

	if err := original.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	loaded, err := LoadEnvFile(original.EnvFilePath())
	if err != nil {
		t.Fatalf("LoadEnvFile() returned unexpected error: %v", err)
	}

	if !reflect.DeepEqual(*loaded, original) {
		t.Errorf("LoadEnvFile() round trip = %+v, want %+v", *loaded, original)
	}
}

func TestEnvFile_ExactFormat(t *testing.T) {
	instance := New(NewParams{
		Project:        "myapp",
		Name:           "my-feature",
		Branch:         "feature/my-feature",
		MainRepository: "/home/user/projects/myapp",
		Path:           t.TempDir(),
		BlockStart:     4640,
		Ports: map[string]int{
			"HTTP_PORT":     4640,
			"DATABASE_PORT": 4641,
		},
	})
	instance.AdvanceState(StateProvisioned)
	instance.LockHashes["composer.lock"] = "deadbeef"

	want := "# Managed by treehouse — do not edit. Regenerate with: treehouse provision\n" +
		"COMPOSE_PROJECT_NAME=myapp-my-feature\n" +
		"TREEHOUSE_PROJECT=myapp\n" +
		"TREEHOUSE_INSTANCE=my-feature\n" +
		"TREEHOUSE_BRANCH=feature/my-feature\n" +
		"TREEHOUSE_MAIN_REPOSITORY=/home/user/projects/myapp\n" +
		"TREEHOUSE_BLOCK=4640\n" +
		"TREEHOUSE_STATE=provisioned\n" +
		"TREEHOUSE_LOCK_HASH_composer.lock=deadbeef\n" +
		"DATABASE_PORT=4641\n" +
		"HTTP_PORT=4640\n"

	if got := instance.render(); got != want {
		t.Errorf("render() = %q, want %q", got, want)
	}
}

func TestLoadEnvFile_SetsPathToContainingDirectory(t *testing.T) {
	dir := t.TempDir()
	instance := New(NewParams{
		Project: "myapp", Name: "feature-a", Branch: "feature-a",
		MainRepository: "/repo", Path: dir, BlockStart: 4000,
		Ports: map[string]int{"HTTP_PORT": 4000},
	})
	if err := instance.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	loaded, err := LoadEnvFile(filepath.Join(dir, EnvFileName))
	if err != nil {
		t.Fatalf("LoadEnvFile() returned unexpected error: %v", err)
	}
	if loaded.Path != dir {
		t.Errorf("Path = %q, want %q", loaded.Path, dir)
	}
}
