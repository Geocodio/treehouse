// Package registry implements the machine-global port registry: a single
// JSON file, guarded by an exclusive flock, that tracks which port block
// belongs to which instance across every treehouse-managed repository on
// the machine. See DESIGN.md's "Port registry" section for the contract.
package registry

import "time"

// Claim is one instance's reservation of a consecutive block of ports.
type Claim struct {
	Project    string    `json:"project"`
	Instance   string    `json:"instance"`
	Repository string    `json:"repository"`
	Worktree   string    `json:"worktree"`
	BlockStart int       `json:"block_start"`
	BlockSize  int       `json:"block_size"`
	CreatedAt  time.Time `json:"created_at"`
}

// blockEnd returns the claim's block end, exclusive.
func (claim Claim) blockEnd() int {
	return claim.BlockStart + claim.BlockSize
}

// overlaps reports whether the given block intersects claim's block.
func (claim Claim) overlaps(blockStart, blockSize int) bool {
	return claim.BlockStart < blockStart+blockSize && blockStart < claim.blockEnd()
}
