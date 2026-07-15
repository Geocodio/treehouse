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
// reserved per instance and the mapping of environment variable name to the
// port's index within that block.
type PortsConfig struct {
	BlockSize int
	Env       map[string]int
}

// ProvisionConfig is the manifest's provision section, all of which is
// optional: shared directories to symlink, mutable directories to
// copy-on-write clone, plain files to copy, and a post-create hook script.
type ProvisionConfig struct {
	Symlink   []string
	Copy      []CopyEntry
	CopyFiles []string
	Hook      string
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
