# treehouse

Treehouse gives every git branch its own worktree with an isolated
docker-compose stack: a unique compose project, unique host ports, cloned
dependencies, and provisioned databases. It's built so multiple developers —
human or AI agent — can work on parallel branches of one repository, each
with a fully running stack, with zero port collisions across every
treehouse-managed repository on the machine.

> **Scope.** Treehouse is built for our own workflow. Issues are welcome and
> small, focused PRs will be considered, but there's no roadmap and no
> commitment to support use cases outside what we need day to day.

## Install

```
# placeholder — release binaries and a install script will land here
```

## Usage

```
treehouse create <branch> [--name N] [--no-start] [--no-provision] [--json] [--no-gc]
treehouse list [--json] [--all-repos] [--no-gc]
treehouse remove [name] [--force] [--keep-volumes]
treehouse path <name>
treehouse up | down | restart | status
treehouse compose <args...>
treehouse exec <service> -- <command...>
treehouse provision [--retry]
treehouse switch
treehouse gc [--idle-hours N] [--dry-run]
treehouse prune [--days N] [--dry-run] [--force]
treehouse init [--write]
treehouse doctor
treehouse version
```

| Command | What it does |
|---|---|
| `create` | Creates (or, per branch, reuses) an instance: claims a port block, adds a worktree, writes `.treehouse.env`, clones dependencies, brings the stack up, then runs the post-create hook — in that order — unless `--no-start`/`--no-provision` stop it early. |
| `list` | Lists instances for the current repository, or every treehouse-managed repository with `--all-repos` (flagging any registry claim whose worktree is gone as `"stale": true`). |
| `remove` | Stops the stack, removes it (and its volumes, unless `--keep-volumes`), removes the worktree, and releases its port claim. Refuses a dirty worktree without `--force`. |
| `path` | Prints an instance's worktree directory, for `cd "$(treehouse path foo)"`. |
| `up` / `down` / `restart` / `status` | Context-aware compose wrappers: instance-scoped inside an instance, plain passthrough in a main repository. |
| `compose` | Raw `docker compose` passthrough, with `--env-file`/`--project-name` injected inside an instance. |
| `exec` | Sugar for `treehouse compose exec <service> <command...>`. |
| `provision` | Brings the stack up if it isn't, then (re-)runs reconcile/hook provisioning; `--retry` re-runs a fully provisioned or provision-failed instance. |
| `switch` | Interactive-only: pick an instance (fzf if installed, numbered menu otherwise) and print its path, for `cd "$(treehouse switch)"`. |
| `gc` | Stops (never removes) instances idle past `--idle-hours` (default 24h) and reaps stale registry claims. Runs opportunistically from `create`/`list` too, unless `--no-gc`. |
| `prune` | Removes instances idle past `--days` (default 30) via the same path as `remove`; a dirty worktree is always skipped, and each removal is confirmed individually. |
| `init` | Proposes a `.treehouse.yml` from `docker-compose.yml`'s published ports; `--write` saves it (refusing to overwrite an existing manifest). Never edits the compose file. |
| `doctor` | Checks manifest validity, the compose file's contract (hardcoded ports/`container_name` fail, absolute bind mounts warn), registry health, and tool availability; exits nonzero on any failing check. |

See [`docs/DESIGN.md`](docs/DESIGN.md) for the full command reference and
implementation contract.

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

## License

MIT, see [LICENSE](LICENSE).
