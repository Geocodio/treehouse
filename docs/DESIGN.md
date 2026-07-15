# Treehouse — Design & Implementation Contract

Treehouse gives every git branch its own worktree with an isolated docker-compose stack:
unique compose project, unique host ports, cloned dependencies, provisioned databases.
Built so multiple developers — human or AI agent — can work on parallel branches of one
repository, each with a fully running stack, with zero port collisions across every
treehouse-managed repository on the machine.

This document is the implementation contract: package layout, behavioral rules, and
invariants. It is the source of truth when code and docs disagree.

## Toolchain and dependencies

- Go 1.25, module `github.com/geocodio/treehouse`
- CLI: `github.com/spf13/cobra`
- YAML: `gopkg.in/yaml.v3`
- File locking: `golang.org/x/sys/unix` (Flock). No other runtime dependencies.
- External tools invoked at runtime: `git`, `docker` (compose v2 plugin; fall back to
  `docker-compose` if the plugin is absent), `cp`, `lsof`. `fzf` is optional (interactive
  switch only; numbered menu fallback).
- macOS and Linux only. Windows is explicitly unsupported.

## Package layout

```
cmd/treehouse/main.go      thin main; wires cobra commands, maps errors to exit codes
internal/cli/              command implementations, TTY detection, JSON output helpers
internal/manifest/         .treehouse.yml parsing + validation
internal/registry/         machine-global port registry (locking, allocation, GC)
internal/gitx/             git worktree operations via git CLI (porcelain parsing)
internal/instance/         instance model: state machine, .treehouse.env read/write
internal/provision/        symlink / CoW copy / lockfile reconciliation / hook runner
internal/compose/          docker compose invocation + label-based container queries
claude/                    Claude Code integration (SessionEnd hook script, skill)
docs/                      user documentation
testdata/fixture/          toy two-service compose project used by integration tests
```

## Naming conventions

Spell identifiers out in full (`instancePath`, not `instPath`; `portBlock`, not `pb`).
Only universally understood shorthands (`id`, `url`, `env`) are acceptable. Comments
document the why, never the what.

## Files and locations

| File | Location | Committed? |
|---|---|---|
| Manifest | `.treehouse.yml` at repo root | yes, by the adopting repo |
| Instance env | `.treehouse.env` at instance worktree root | no (repos gitignore it) |
| Port registry | `${XDG_STATE_HOME:-~/.local/state}/treehouse/registry.json` + `registry.lock` | machine-local |
| Machine config | `${XDG_CONFIG_HOME:-~/.config}/treehouse/config.yml` | machine-local |
| Instances home | `~/treehouses/<project>/<instance>/` (default; overridable in machine config and per-manifest) | n/a |

## Manifest schema (version 1)

```yaml
version: 1                     # required; reject greater versions with a clear error
project: myapp                 # required; compose project prefix; [a-z0-9][a-z0-9_-]*

ports:
  block_size: 32               # default 32
  env:                         # ENV_VAR -> index within block
    HTTP_PORT: 0
    DATABASE_PORT: 1
    MAIL_UI_PORT: 2

provision:                     # all optional
  symlink:                     # shared directories, symlinked to the main repo's copy
    - data/shared
  copy:                        # per-instance mutable directories, CoW-cloned
    - path: vendor
      reconcile:               # optional: re-run installer only when lockfile drifted
        lockfile: composer.lock
        command: composer install
    - path: node_modules
      reconcile:
        lockfile: package-lock.json
        command: npm install   # never `npm ci` — it deletes the cloned directory
    - public/build             # plain string form = copy without reconciliation
  copy_files:
    - .env.docker
  hook: scripts/treehouse-post-create.sh

urls:                          # optional; human-facing summary lines
  - name: App
    url: "http://localhost:{HTTP_PORT}"

location: ~/treehouses         # optional override of instances home
```

Validation rules (all produce actionable error messages):
- every `ports.env` index must be unique and `< block_size`
- `project` must match the compose project-name charset
- `hook`, `symlink`, `copy`, `copy_files` paths must be relative (no `..` escapes)
- unknown top-level keys are an error (typo protection), unknown nested keys a warning

## Port registry

A single JSON file guarded by an exclusive `flock` on `registry.lock` for every
read-modify-write. Schema:

```json
{
  "version": 1,
  "claims": [
    {
      "project": "myapp",
      "instance": "my-feature",
      "repository": "/home/user/projects/myapp",
      "worktree": "/home/user/treehouses/myapp/my-feature",
      "block_start": 4640,
      "block_size": 32,
      "created_at": "2026-07-15T12:00:00Z"
    }
  ]
}
```

Allocation algorithm:
1. Take the lock.
2. Garbage-collect stale claims (worktree path no longer exists).
3. Scan the pool (default 4000–9999, machine-configurable) in `block_size` steps for the
   first block that (a) overlaps no existing claim and (b) has no port currently bound
   per `lsof -nP -iTCP -sTCP:LISTEN` (single invocation, parsed once — never one `lsof`
   per port).
4. Write the claim, release the lock.

Invariant: every port an instance publishes lives inside its own claimed block. Cross-
instance collisions are therefore structurally impossible; no cleverness required.
`lsof` absence degrades to registry-only checking with a warning, not a failure.

## Instance model

Identity: **one instance per branch per repository.** Instance name defaults to the
branch slug (last path segment, lowercased, `[^a-z0-9_-]` → `-`). A slug collision with
a *different* branch auto-suffixes (`foo-2`) with a warning.

`.treehouse.env` (written atomically; both a docker-compose env file and the dispatch
marker for repo tooling):

```
# Managed by treehouse — do not edit. Regenerate with: treehouse provision
COMPOSE_PROJECT_NAME=myapp-my-feature
TREEHOUSE_PROJECT=myapp
TREEHOUSE_INSTANCE=my-feature
TREEHOUSE_BRANCH=feature/my-feature
TREEHOUSE_MAIN_REPOSITORY=/home/user/projects/myapp
TREEHOUSE_BLOCK=4640
TREEHOUSE_STATE=provisioned
TREEHOUSE_LOCK_HASH_composer.lock=<sha256 of main repo's lockfile at clone time>
HTTP_PORT=4640
DATABASE_PORT=4641
```

State machine, recorded in `TREEHOUSE_STATE`:
`worktree-created → deps-cloned → stack-up → provisioned`, plus `provision-failed`.
No automatic rollback: a failed hook leaves the instance inspectable; `treehouse
provision --retry` re-runs from the failed step. Hook idempotency is the adopting
repository's contract, documented in the adoption guide.

## Command behavior

```
treehouse create <branch> [--name N] [--no-start] [--no-provision] [--json]
treehouse list [--json] [--all-repos]
treehouse remove [name] [--force] [--keep-volumes]
treehouse up | down | restart | status
treehouse compose <args...>
treehouse exec <service> -- <command...>
treehouse provision [--retry]
treehouse switch
treehouse path <name>
treehouse gc [--idle-hours N] [--dry-run]
treehouse prune [--days N] [--dry-run] [--force]
treehouse init
treehouse doctor
treehouse version
```

Behavioral rules:
- **create is idempotent per branch.** If an instance for the branch exists: exit 0,
  print/emit the existing instance (JSON gains `"existing": true`), and `up` it if
  stopped. Branch doesn't exist locally: create it from the current branch (tracking
  branch if it exists on origin).
- **Every create/list invocation opportunistically runs gc** (idle-stop, never remove)
  unless `--no-gc` or gc ran within the last hour (timestamp in registry file).
- **remove/prune never destroy dirty worktrees silently.** Attempt `git worktree remove`
  *without* `--force`; on refusal show `git status --short` and require the user's
  explicit `--force`. Prune additionally treats any dirty tree as in-use regardless of
  age. Remove releases the port claim and removes the compose stack's volumes by
  default (`--keep-volumes` to retain).
- **compose passthrough**: inside an instance, inject
  `--env-file .treehouse.env --project-name $COMPOSE_PROJECT_NAME`; in a main repo,
  pass through unchanged. All internal compose calls go through this same path.
- **Container queries use the `com.docker.compose.project` label exclusively.** Never
  match container names by substring.
- **up re-verifies** the port block against listening sockets before starting and fails
  with a specific error naming the squatting process if occupied.
- **gc idle detection**: an instance is idle when its newest signal — container activity
  (docker events are not tracked; use container `State.FinishedAt`/`StartedAt` and
  process stats), git HEAD reflog mtime, and mtime of tracked-file changes — is older
  than the threshold (default 24h). gc stops containers; it never touches files.
- **exec** is sugar: `treehouse compose exec <service> <command...>`.
- **path** prints the instance directory (for `cd "$(treehouse path foo)"`).
- **switch** is interactive-only: fzf when present, numbered menu otherwise; refuses to
  run without a TTY.
- **init** parses `docker-compose.yml`, extracts published ports, proposes a manifest
  with `${VAR:-default}` substitutions and prints the compose diff needed. It never
  edits the compose file.
- **doctor** checks: manifest validity; compose contract (hardcoded published ports,
  `container_name` present, absolute bind mounts flagged as warnings); registry health
  (stale claims, orphaned instances); tool availability (docker, git versions).

## Agent mode (first-class)

- No TTY on stdin ⇒ non-interactive: no prompts ever; anything that would prompt takes
  its default or fails with a distinct exit code instead.
- `--json` on create/list emits a single JSON document on stdout; all human chatter goes
  to stderr.
- Exit codes are contractual and documented in README:
  `0` success · `1` generic failure · `2` port pool exhausted · `3` dirty-tree refusal ·
  `4` provision failed · `5` not in a treehouse-enabled repository · `6` docker
  unavailable · `7` would-prompt in non-interactive mode.

## Copy-on-write strategy

- macOS/APFS: `cp -Rc` (clonefile). Linux: `cp -a --reflink=auto` (instant on XFS/btrfs,
  full copy on ext4). Always via `cp`, never a Go file-walk — the syscall fast paths are
  the point.
- After cloning a `copy` entry with `reconcile`, record the sha256 of the main repo's
  lockfile in `.treehouse.env`. Provisioning compares the instance's lockfile against
  the recorded hash and runs `reconcile.command` inside the instance only on mismatch.

## Recommended repo integration (documented, not enforced)

Adopting repositories keep plain `docker compose` fully working in the main checkout.
The recommended Taskfile/Makefile pattern dispatches on the instance marker file:

```yaml
vars:
  COMPOSE:
    sh: test -f .treehouse.env && echo "treehouse compose" || echo "docker compose"
```

Treehouse is required only where an instance exists — and instances can only be created
by treehouse, so the dependency is self-consistent and strictly opt-in.

## Testing

- Unit: manifest validation, registry allocation (property-style: no two concurrent
  claims overlap; GC reclaims), slug derivation, env-file round-trip, lsof parsing.
- Integration: full lifecycle against `testdata/fixture` (create → up → exec → down →
  remove) behind a build tag (`integration`) since it needs docker; CI runs unit tests
  always, integration on a docker-enabled runner.
- The registry lock must be exercised by a concurrent-allocation test.

## Out of scope for v1

Volume-template database cloning, reverse-proxy/DNS-based per-instance URLs (candidate
for v2), background daemons, remote instances, Windows, multi-repo compositions.
