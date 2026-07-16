// Package manifest parses and validates .treehouse.yml, the per-repository
// configuration file described in DESIGN.md's "Manifest schema" section.
package manifest

// CurrentVersion is the highest manifest schema version this build of
// treehouse understands. Manifests declaring a higher version are rejected
// outright rather than partially parsed, since a newer schema may have
// changed the meaning of existing keys.
const CurrentVersion = 1

// DefaultBlockSize is the number of consecutive ports reserved per instance
// when a manifest does not set ports.block_size.
const DefaultBlockSize = 32

// Manifest is the fully parsed and validated contents of .treehouse.yml.
type Manifest struct {
	Version   int
	Project   string
	Ports     PortsConfig
	Provision ProvisionConfig
	URLs      []URL
	Location  string
}

// PortsConfig is the manifest's ports section: the size of the block
// reserved per instance, the mapping of environment variable name to the
// port's index within that block, and any alias environment variables that
// should be written to .treehouse.env with the same value as a canonical
// ports.env variable (some apps read the same published port under several
// env var names).
type PortsConfig struct {
	BlockSize int
	Env       map[string]int
	Aliases   map[string]string
}

// ProvisionConfig is the manifest's provision section, all of which is
// optional: shared directories to symlink, mutable directories to
// copy-on-write clone, plain files to copy, and a post-create hook script.
type ProvisionConfig struct {
	Symlink   []SymlinkEntry
	Copy      []CopyEntry
	CopyFiles []string
	Hook      string
}

// SymlinkEntry is one entry of provision.symlink. It accepts either the
// plain string form (a required shared path whose missing source is a hard
// failure) or the struct form pairing a path with `optional: true`, which
// downgrades a missing source to a warn-and-skip — for a symlink target the
// main repository may legitimately not have generated yet (dev certs, a
// build artifact directory).
type SymlinkEntry struct {
	Path     string
	Optional bool
}

// CopyEntry is one entry of provision.copy. It accepts either the plain
// string form (copy without reconciliation) or the struct form pairing a
// path with an optional reconcile step.
type CopyEntry struct {
	Path      string
	Reconcile *ReconcileConfig
}

// ReconcileConfig re-runs an installer command inside the instance only when
// the main repository's lockfile has drifted since the directory was cloned.
type ReconcileConfig struct {
	Lockfile string
	Command  string
}

// URL is one entry of the manifest's urls section: a human-facing summary
// line printed after an instance comes up.
type URL struct {
	Name string
	URL  string
}
