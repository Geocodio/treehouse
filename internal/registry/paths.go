package registry

import (
	"fmt"
	"os"
	"path/filepath"
)

// StateDir resolves the machine-local directory treehouse stores its port
// registry in: $XDG_STATE_HOME/treehouse, defaulting to
// ~/.local/state/treehouse when XDG_STATE_HOME is unset, per the XDG Base
// Directory spec and DESIGN.md's "Files and locations" table.
func StateDir() (string, error) {
	if xdgStateHome := os.Getenv("XDG_STATE_HOME"); xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "treehouse"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}

	return filepath.Join(homeDir, ".local", "state", "treehouse"), nil
}
