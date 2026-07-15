package instance

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// invalidSlugCharacters matches every character Slugify must fold into a
// hyphen: anything other than a lowercase letter, digit, underscore, or
// hyphen.
var invalidSlugCharacters = regexp.MustCompile(`[^a-z0-9_-]`)

// fallbackSlug is used when a branch name slugifies to nothing at all
// (e.g. a branch literally named "///" or "!!!").
const fallbackSlug = "instance"

// Slugify derives an instance name from a branch name: its last path
// segment, lowercased, with every character outside [a-z0-9_-] folded to a
// hyphen, then leading/trailing hyphens trimmed. Per DESIGN.md: "Instance
// name defaults to the branch slug (last path segment, lowercased,
// [^a-z0-9_-] -> -)".
func Slugify(branch string) string {
	segment := path.Base(branch)
	lowered := strings.ToLower(segment)
	folded := invalidSlugCharacters.ReplaceAllString(lowered, "-")
	trimmed := strings.Trim(folded, "-")

	if trimmed == "" {
		return fallbackSlug
	}

	return trimmed
}

// ResolveName computes the instance name to use for branch given the
// instances that already exist for this repository.
//
// If branch already has an instance among existing, its name is returned
// unchanged so `create` stays idempotent per branch. Otherwise the base
// slug is used unless it collides with a *different* branch's existing
// instance, in which case a numeric suffix (-2, -3, ...) is appended until
// the name is free, and warning explains why.
func ResolveName(existing []Instance, branch string) (name string, warning string) {
	for _, instance := range existing {
		if instance.Branch == branch {
			return instance.Name, ""
		}
	}

	takenByOtherBranch := make(map[string]bool, len(existing))
	for _, instance := range existing {
		takenByOtherBranch[instance.Name] = true
	}

	base := Slugify(branch)
	if !takenByOtherBranch[base] {
		return base, ""
	}

	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !takenByOtherBranch[candidate] {
			return candidate, fmt.Sprintf(
				"instance name %q is already used by another branch; using %q instead",
				base, candidate,
			)
		}
	}
}
