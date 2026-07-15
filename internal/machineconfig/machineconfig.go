// Package machineconfig loads the machine-local treehouse configuration
// file described in DESIGN.md's "Files and locations" table:
// ${XDG_CONFIG_HOME:-~/.config}/treehouse/config.yml. Every key is
// optional; a missing file is not an error, it simply yields defaults.
package machineconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultInstancesHomeName is the directory name used under the user's home
// directory when neither the machine config nor a manifest overrides it,
// per DESIGN.md's "~/treehouses (default; overridable...)" rule.
const DefaultInstancesHomeName = "treehouses"

// Config is the fully resolved machine-local configuration: instances_home
// and pool_start/pool_end default to their documented values when the
// config file omits them or is absent entirely.
type Config struct {
	InstancesHome string
	PoolStart     int
	PoolEnd       int
}

// rawConfig mirrors config.yml's on-disk shape before defaults are applied
// and "~" is expanded.
type rawConfig struct {
	InstancesHome string `yaml:"instances_home"`
	PoolStart     int    `yaml:"pool_start"`
	PoolEnd       int    `yaml:"pool_end"`
}

// Path returns the config file's resolved location:
// $XDG_CONFIG_HOME/treehouse/config.yml, defaulting to
// ~/.config/treehouse/config.yml.
func Path() (string, error) {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "treehouse", "config.yml"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "treehouse", "config.yml"), nil
}

// Load reads and parses the machine config, applying defaults (from
// poolStartDefault/poolEndDefault, typically registry.DefaultPoolStart/
// DefaultPoolEnd) for any key the file omits or doesn't exist at all. A
// missing config file is not an error.
func Load(poolStartDefault, poolEndDefault int) (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	config := &Config{
		InstancesHome: defaultInstancesHome(),
		PoolStart:     poolStartDefault,
		PoolEnd:       poolEndDefault,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading machine config %s: %w", path, err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing machine config %s: %w", path, err)
	}

	if raw.InstancesHome != "" {
		config.InstancesHome = ExpandHome(raw.InstancesHome)
	}
	if raw.PoolStart != 0 {
		config.PoolStart = raw.PoolStart
	}
	if raw.PoolEnd != 0 {
		config.PoolEnd = raw.PoolEnd
	}

	return config, nil
}

func defaultInstancesHome() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return DefaultInstancesHomeName
	}

	return filepath.Join(homeDir, DefaultInstancesHomeName)
}

// ExpandHome expands a leading "~" or "~/" in path to the current user's
// home directory. Paths not starting with "~" are returned unchanged. This
// is shared by machineconfig's instances_home key and the manifest's
// location override, both of which accept "~/treehouses"-style paths.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if path == "~" {
		return homeDir
	}

	return filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
}
