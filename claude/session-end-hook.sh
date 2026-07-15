#!/bin/sh
# Claude Code SessionEnd hook: stop a treehouse instance's compose stack when
# a session ends, so idle containers don't keep running between sessions.
#
# Contract (see docs/adoption.md for how this gets wired into a repo):
#   - Stops containers only (`treehouse down`). Never removes a worktree,
#     a port claim, or volumes — that is `treehouse remove`, an explicit,
#     user-approved decision, never something a cleanup hook does on its own.
#   - Silently does nothing if the session isn't inside a treehouse instance,
#     or if the `treehouse` binary isn't on PATH. There is nothing to clean
#     up in either case.
#   - Never exits nonzero. A SessionEnd hook that fails must not be allowed
#     to break session teardown itself, so every code path below ends in
#     `exit 0` regardless of what `treehouse down` itself does.
set -u

# find_treehouse_root walks upward from $1 looking for .treehouse.env,
# mirroring internal/instance.FindEnvFileUpwards: the starting directory or
# one of its ancestors, stopping at the filesystem root. Prints the
# directory and returns 0 on a match; returns 1 with no output otherwise.
find_treehouse_root() {
	directory=$1

	while true; do
		if [ -f "$directory/.treehouse.env" ]; then
			printf '%s\n' "$directory"
			return 0
		fi

		parent=$(dirname "$directory")
		if [ "$parent" = "$directory" ]; then
			return 1
		fi
		directory=$parent
	done
}

# CLAUDE_PROJECT_DIR is set by Claude Code to the session's project root;
# fall back to the current working directory so this script still behaves
# sensibly when run outside a Claude Code session (e.g. manual testing).
start_directory="${CLAUDE_PROJECT_DIR:-$(pwd)}"

instance_directory=$(find_treehouse_root "$start_directory") || exit 0

command -v treehouse >/dev/null 2>&1 || exit 0

(cd "$instance_directory" && treehouse down) >/dev/null 2>&1

exit 0
