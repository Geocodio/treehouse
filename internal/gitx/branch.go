package gitx

import "os/exec"

// BranchExistsLocal reports whether branch exists as a local branch in the
// repository at dir.
func BranchExistsLocal(dir, branch string) (bool, error) {
	return refExists(dir, "refs/heads/"+branch)
}

// BranchExistsRemote reports whether branch exists on the "origin" remote,
// per the local remote-tracking ref refs/remotes/origin/<branch>. This is a
// local, no-network check: it reflects the last fetch, which matches how
// `create` decides whether to track an existing remote branch.
func BranchExistsRemote(dir, branch string) (bool, error) {
	return refExists(dir, "refs/remotes/origin/"+branch)
}

func refExists(dir, ref string) (bool, error) {
	command := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	command.Dir = dir

	if err := command.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// show-ref exits 1 when the ref simply doesn't exist; that's a
			// negative result, not a failure.
			if exitErr.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, err
	}

	return true, nil
}

// CreateBranch creates branch in the repository at dir. If branch already
// exists on the "origin" remote, the new local branch tracks
// origin/<branch> (checking it out at the same commit as the remote
// branch); otherwise it is created from the current branch's tip with no
// tracking configured, per DESIGN.md's create command: "create it from the
// current branch (tracking branch if it exists on origin)".
func CreateBranch(dir, branch string) error {
	hasRemote, err := BranchExistsRemote(dir, branch)
	if err != nil {
		return err
	}

	if hasRemote {
		_, err := run(dir, "branch", "--track", branch, "origin/"+branch)
		return err
	}

	_, err = run(dir, "branch", branch)
	return err
}
