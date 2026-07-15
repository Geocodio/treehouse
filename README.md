# treehouse

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Treehouse gives every git branch its own worktree with an isolated
docker-compose stack: a unique compose project, unique host ports, cloned
dependencies, and provisioned databases. It's built so multiple developers
— human or AI agent — can work on parallel branches of one repository, each
with a fully running stack, with zero port collisions across every
treehouse-managed repository on the machine.

> **Scope.** Treehouse is built for our own workflow. Issues are welcome and
> small, focused PRs will be considered, but there's no roadmap and no
> commitment to support use cases outside what we need day to day.

## Install

**Homebrew** (macOS/Linux):

```
brew tap geocodio/tap
brew trust geocodio/tap   # newer Homebrew requires trusting third-party taps once
brew install treehouse
```

**`go install`:**

```
go install github.com/geocodio/treehouse/cmd/treehouse@latest
```

**Build from source:**

```
git clone https://github.com/geocodio/treehouse.git
cd treehouse
make build    # ./treehouse, version stamped from `git describe`
make install  # or: go install ./cmd/treehouse
```

Requires macOS or Linux (Windows is explicitly unsupported), `git`, and
`docker` with the `compose` plugin (or the standalone `docker-compose`
binary as a fallback). `fzf` is optional — `treehouse switch` falls back to
a numbered menu without it.

## Quickstart

Inside a repository that has adopted treehouse (see
[`docs/adoption.md`](docs/adoption.md) if yours hasn't yet):

```
treehouse init --write                          # first time only: propose + save .treehouse.yml
treehouse create feature/my-feature --json       # claim ports, worktree, deps, stack up, hook
cd "$(treehouse path my-feature)"
treehouse exec web -- pytest                     # or whatever your test command is
treehouse remove my-feature --force              # when you're done with it
```

`create` is idempotent per branch — running it again for the same branch
just reports the existing instance and brings its stack up if it was
stopped, rather than erroring or creating a second one.

## Commands

| Command | Flags | What it does |
|---|---|---|
| `create <branch>` | `--name N`, `--no-start`, `--no-provision`, `--json`, `--no-gc` | Creates (or, per branch, reuses) an instance: claims a port block, adds a worktree, writes `.treehouse.env`, clones dependencies, brings the stack up, then runs the post-create hook — in that order — unless `--no-start`/`--no-provision` stop it early. `--name` sets an explicit instance name instead of deriving one from the branch. |
| `list` | `--json`, `--all-repos`, `--no-gc` | Lists instances for the current repository, or every treehouse-managed repository with `--all-repos` (flagging any registry claim whose worktree is gone as `"stale": true`). |
| `remove [name]` | `--force`, `--keep-volumes` | Stops the stack, removes it (and its volumes, unless `--keep-volumes`), removes the worktree, and releases its port claim. Refuses a dirty worktree, and refuses to prompt outside a terminal, without `--force`. Omit `name` to remove the instance containing the current directory. |
| `path <name>` | | Prints an instance's worktree directory, for `cd "$(treehouse path foo)"`. |
| `up` | | Brings the compose stack up; instance-scoped inside an instance (re-verifying its port block first), plain passthrough in a main repository. |
| `down` | `--keep-volumes` | Brings the compose stack down (removing named volumes unless `--keep-volumes`). |
| `restart` | | Restarts the compose stack. |
| `status` | | Shows the instance's state and running container count (or plain `docker compose ps` in a main repository). |
| `compose -- <args...>` | | Raw `docker compose` passthrough, with `--env-file`/`--project-name` injected inside an instance. |
| `exec <service> -- <command...>` | | Sugar for `treehouse compose exec <service> <command...>`. |
| `provision` | `--retry` | Brings the stack up if it isn't, then (re-)runs reconcile/hook provisioning; completes an instance created with `--no-start`/`--no-provision` without needing `--retry`. `--retry` re-runs one already `provisioned` (or `provision-failed`). |
| `switch` | | Interactive-only: pick an instance (fzf if installed, numbered menu otherwise) and print its path, for `cd "$(treehouse switch)"`. Refuses to run without a TTY. |
| `gc` | `--idle-hours N` (default `24`), `--dry-run` | Stops (never removes) instances idle past the threshold and reaps stale registry claims. Runs opportunistically from `create`/`list` too (at most once an hour), unless `--no-gc`. |
| `prune` | `--days N` (default `30`), `--dry-run`, `--force` | Removes instances idle past the threshold via the same path as `remove`; a dirty worktree is always skipped, and each removal is confirmed individually — `--force` only skips that per-instance prompt, never the dirty-tree refusal. |
| `init` | `--write` | Proposes a `.treehouse.yml` from `docker-compose.yml`'s published ports, plus the compose diff needed to parameterize any hardcoded port. `--write` saves it (refusing to overwrite an existing manifest). Never edits the compose file. |
| `doctor` | | Checks manifest validity, the compose file's contract (hardcoded ports/`container_name` fail, absolute bind mounts warn), registry health, and tool availability; exits nonzero on any failing check. |
| `version` | | Prints the treehouse version. |

See [`docs/DESIGN.md`](docs/DESIGN.md) for the full implementation contract
and behavioral rules behind this table.

## Exit codes

Exit codes are part of treehouse's contract with scripts and agents — they
will not be renumbered or repurposed:

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Generic failure |
| `2` | Port pool exhausted |
| `3` | Dirty-tree refusal |
| `4` | Provision failed |
| `5` | Not in a treehouse-enabled repository |
| `6` | Docker unavailable |
| `7` | Would-prompt in non-interactive mode |

No TTY on stdin means non-interactive: nothing ever prompts — anything
that would takes its documented default or fails with exit `7` instead.
`--json` (on `create`/`list`) emits a single JSON document on stdout; all
human-readable chatter, including warnings, goes to stderr.

## Documentation

- [`docs/adoption.md`](docs/adoption.md) — step-by-step guide to adopting
  treehouse in an existing docker-compose project: `treehouse init`,
  fixing the compose contract, writing the post-create hook, and the
  `docker compose`/`treehouse compose` dispatch pattern.
- [`docs/manifest.md`](docs/manifest.md) — full reference for
  `.treehouse.yml`, the generated `.treehouse.env`, and the machine-local
  `config.yml`.
- [`docs/ports.md`](docs/ports.md) — the port allocation model: why
  collisions are structurally impossible, the registry, and gc/pruning.
- [`docs/DESIGN.md`](docs/DESIGN.md) — the full implementation contract:
  package layout, the instance state machine, and every command's
  behavioral rules. Source of truth when code and other docs disagree.
- [`claude/README.md`](claude/README.md) — optional Claude Code
  integration: a SessionEnd hook that stops an instance's stack when a
  session ends, and a skill that teaches an agent to use an
  already-adopted repository's treehouse setup safely.

## License

MIT, see [LICENSE](LICENSE).
