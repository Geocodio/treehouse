package cli

import (
	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/machineconfig"
	"github.com/geocodio/treehouse/internal/manifest"
)

// resolveInstancesHome applies DESIGN.md's override precedence: a
// manifest's `location` key wins over the machine config's
// instances_home, which itself defaults to ~/treehouses.
func resolveInstancesHome(manifestConfig *manifest.Manifest, config *machineconfig.Config) string {
	if manifestConfig.Location != "" {
		return machineconfig.ExpandHome(manifestConfig.Location)
	}

	return config.InstancesHome
}

// findByBranch returns the instance already tracking branch, if any, so
// `create` can honor its "idempotent per branch" contract.
func findByBranch(instances []instance.Instance, branch string) (instance.Instance, bool) {
	for _, candidate := range instances {
		if candidate.Branch == branch {
			return candidate, true
		}
	}

	return instance.Instance{}, false
}

// collidesWithDifferentBranch reports whether name is already used by an
// instance tracking a different branch — a hard error for an explicit
// --name (unlike the auto-derived slug case, which auto-suffixes instead).
func collidesWithDifferentBranch(instances []instance.Instance, name, branch string) bool {
	for _, candidate := range instances {
		if candidate.Name == name && candidate.Branch != branch {
			return true
		}
	}

	return false
}
