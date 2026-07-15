package manifest

import "strings"

// Warning is a non-fatal problem found while parsing the manifest, such as
// an unknown key nested inside a known section. Per DESIGN.md, unknown
// nested keys warn rather than fail so that forward-compatible manifests
// (e.g. authored against a newer treehouse version) still load.
type Warning struct {
	// Key is the dotted path to the offending key, e.g. "ports.bloc_size".
	Key string

	// Message explains the problem in a way suitable for direct display.
	Message string
}

func (warning Warning) String() string {
	return warning.Key + ": " + warning.Message
}

// ValidationError aggregates every fatal problem found while parsing a
// manifest so a user can fix them all in one pass instead of one error at a
// time.
type ValidationError struct {
	Problems []string
}

func (validationError *ValidationError) Error() string {
	return "invalid manifest:\n  " + strings.Join(validationError.Problems, "\n  ")
}
