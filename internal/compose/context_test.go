package compose

import (
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
)

func TestResolveInstance_MainRepositoryReturnsNil(t *testing.T) {
	inst, err := ResolveInstance(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveInstance() returned unexpected error: %v", err)
	}
	if inst != nil {
		t.Errorf("ResolveInstance() = %+v, want nil for a directory outside any instance", inst)
	}
}

func TestResolveInstance_LoadsInstanceFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	written := instance.New(instance.NewParams{
		Project: "myapp", Name: "feature-a", Branch: "feature-a",
		MainRepository: "/repo", Path: dir, BlockStart: 4000,
		Ports: map[string]int{"HTTP_PORT": 4000},
	})
	if err := written.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	loaded, err := ResolveInstance(dir)
	if err != nil {
		t.Fatalf("ResolveInstance() returned unexpected error: %v", err)
	}
	if loaded == nil {
		t.Fatalf("ResolveInstance() = nil, want the loaded instance")
	}
	if loaded.Name != "feature-a" {
		t.Errorf("ResolveInstance().Name = %q, want feature-a", loaded.Name)
	}
}
