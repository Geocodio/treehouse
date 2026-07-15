package machineconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileYieldsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	config, err := Load(4000, 9999)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if config.PoolStart != 4000 || config.PoolEnd != 9999 {
		t.Errorf("Load() pool = [%d,%d], want defaults [4000,9999]", config.PoolStart, config.PoolEnd)
	}

	homeDir, _ := os.UserHomeDir()
	want := filepath.Join(homeDir, DefaultInstancesHomeName)
	if config.InstancesHome != want {
		t.Errorf("InstancesHome = %q, want %q", config.InstancesHome, want)
	}
}

func TestLoad_ParsesOverrides(t *testing.T) {
	xdgConfigHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	configDir := filepath.Join(xdgConfigHome, "treehouse")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	contents := "instances_home: /custom/treehouses\npool_start: 5000\npool_end: 5999\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("writing config.yml: %v", err)
	}

	config, err := Load(4000, 9999)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if config.InstancesHome != "/custom/treehouses" {
		t.Errorf("InstancesHome = %q, want /custom/treehouses", config.InstancesHome)
	}
	if config.PoolStart != 5000 || config.PoolEnd != 5999 {
		t.Errorf("pool = [%d,%d], want [5000,5999]", config.PoolStart, config.PoolEnd)
	}
}

func TestLoad_PartialOverrideKeepsOtherDefaults(t *testing.T) {
	xdgConfigHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	configDir := filepath.Join(xdgConfigHome, "treehouse")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.yml"), []byte("pool_start: 5000\n"), 0o644); err != nil {
		t.Fatalf("writing config.yml: %v", err)
	}

	config, err := Load(4000, 9999)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if config.PoolStart != 5000 {
		t.Errorf("PoolStart = %d, want 5000", config.PoolStart)
	}
	if config.PoolEnd != 9999 {
		t.Errorf("PoolEnd = %d, want default 9999", config.PoolEnd)
	}
}

func TestExpandHome(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir(): %v", err)
	}

	tests := []struct {
		path string
		want string
	}{
		{"~/treehouses", filepath.Join(homeDir, "treehouses")},
		{"~", homeDir},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, test := range tests {
		if got := ExpandHome(test.path); got != test.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}
