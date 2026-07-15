package manifest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// projectNamePattern is the compose project-name charset from DESIGN.md's
// manifest schema: a lowercase letter or digit, followed by lowercase
// letters, digits, underscores, or hyphens.
var projectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// environmentVariableNamePattern accepts the identifier syntax POSIX shells
// and docker-compose env files require for a variable name.
var environmentVariableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateProjectName(name string) error {
	if !projectNamePattern.MatchString(name) {
		return fmt.Errorf("project %q must match %s (lowercase letters, digits, underscore, hyphen; cannot start with underscore or hyphen)", name, projectNamePattern.String())
	}

	return nil
}

func validateEnvironmentVariableName(key string) error {
	if !environmentVariableNamePattern.MatchString(key) {
		return fmt.Errorf("ports.env key %q is not a valid environment variable name", key)
	}

	return nil
}

// validateRelativePath enforces the manifest schema's "must be relative (no
// .. escapes)" rule for hook, symlink, copy, and copy_files paths. key
// identifies the offending manifest field in the returned error so the user
// knows exactly what to fix.
func validateRelativePath(key, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s: path must not be empty", key)
	}

	if filepath.IsAbs(path) {
		return fmt.Errorf("%s: path %q must be relative", key, path)
	}

	cleaned := filepath.Clean(path)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s: path %q escapes the repository root", key, path)
	}

	return nil
}
