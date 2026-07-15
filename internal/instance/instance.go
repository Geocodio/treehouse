// Package instance models a treehouse instance: one branch's worktree, its
// claimed port block, its provisioning state, and its .treehouse.env file.
// See DESIGN.md's "Instance model" section for the full contract.
package instance

// State is one value of TREEHOUSE_STATE, per DESIGN.md's state machine:
// worktree-created -> deps-cloned -> stack-up -> provisioned, plus
// provision-failed.
type State string

const (
	StateWorktreeCreated State = "worktree-created"
	StateDepsCloned      State = "deps-cloned"
	StateStackUp         State = "stack-up"
	StateProvisioned     State = "provisioned"
	StateProvisionFailed State = "provision-failed"
)

// stateRank orders the happy-path states so state transitions can be
// applied as a high-water mark rather than blindly overwritten: an already
// -provisioned instance must not regress to stack-up just because `up` ran
// again. provision-failed is deliberately excluded — it is a sentinel that
// always overwrites (a hook can fail after any prior state) and is always
// escapable by a subsequent successful retry, not a rank to compare against.
var stateRank = map[State]int{
	StateWorktreeCreated: 0,
	StateDepsCloned:      1,
	StateStackUp:         2,
	StateProvisioned:     3,
}

// Instance is one branch's treehouse-managed worktree and everything
// .treehouse.env records about it.
type Instance struct {
	// Name is the instance's slug: the branch slug, possibly auto
	// -suffixed on collision. It is also treehouse's identifier for the
	// instance within Project (see registry.Claim).
	Name string

	// Branch is the full branch name (e.g. "feature/my-feature"), as
	// opposed to Name which is derived from just its last path segment.
	Branch string

	// Project is the manifest's project name.
	Project string

	// MainRepository is the absolute path to the repository's main
	// worktree (never a linked/instance worktree).
	MainRepository string

	// Path is the absolute path to this instance's own worktree.
	Path string

	// ComposeProject is the docker-compose project name: "<project>-<name>".
	ComposeProject string

	// BlockStart is the first port of this instance's claimed block.
	BlockStart int

	// State is the current TREEHOUSE_STATE value.
	State State

	// LockHashes maps a reconcile lockfile's relative path (e.g.
	// "composer.lock") to the sha256 hex digest recorded the last time it
	// was cloned or reconciled.
	LockHashes map[string]string

	// Ports maps each manifest ports.env variable name to its absolute
	// port number (BlockStart + the manifest's configured index).
	Ports map[string]int
}

// NewParams are the inputs New needs to build a freshly created instance,
// before it has ever been written to disk.
type NewParams struct {
	Project        string
	Name           string
	Branch         string
	MainRepository string
	Path           string
	BlockStart     int
	Ports          map[string]int
}

// New builds the Instance for a brand new worktree, in StateWorktreeCreated
// — the state `create` records immediately after `git worktree add`
// succeeds, before any provisioning has run.
func New(params NewParams) Instance {
	return Instance{
		Name:           params.Name,
		Branch:         params.Branch,
		Project:        params.Project,
		MainRepository: params.MainRepository,
		Path:           params.Path,
		ComposeProject: params.Project + "-" + params.Name,
		BlockStart:     params.BlockStart,
		State:          StateWorktreeCreated,
		LockHashes:     map[string]string{},
		Ports:          params.Ports,
	}
}

// PortsForBlock computes the absolute port for every manifest ports.env
// entry given the instance's claimed blockStart.
func PortsForBlock(blockStart int, envIndices map[string]int) map[string]int {
	ports := make(map[string]int, len(envIndices))
	for name, index := range envIndices {
		ports[name] = blockStart + index
	}

	return ports
}

// AdvanceState sets the instance's state to next, except that a
// provision-failed state is never silently downgraded back onto the happy
// -path rank order, and the happy-path rank order itself never regresses
// (see stateRank). ProvisionFailed itself always applies immediately,
// since a hook can fail from any prior state.
func (instance *Instance) AdvanceState(next State) {
	if next == StateProvisionFailed {
		instance.State = next
		return
	}

	nextRank, nextKnown := stateRank[next]
	currentRank, currentKnown := stateRank[instance.State]
	if nextKnown && currentKnown && nextRank <= currentRank {
		return
	}

	instance.State = next
}
