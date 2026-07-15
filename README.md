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
treehouse create <branch> [--name N] [--no-start] [--no-provision] [--json]
treehouse list [--json] [--all-repos]
treehouse remove [name] [--force] [--keep-volumes]
treehouse path <name>
treehouse up | down | restart | status
treehouse compose <args...>
treehouse exec <service> -- <command...>
treehouse provision [--retry]
treehouse version
```

| Command | What it does |
|---|---|
| `create` | Creates (or, per branch, reuses) an instance: claims a port block, adds a worktree, writes `.treehouse.env`, provisions, and brings the stack up unless `--no-start`. |
| `list` | Lists instances for the current repository, or every treehouse-managed repository with `--all-repos`. |
| `remove` | Stops the stack, removes it (and its volumes, unless `--keep-volumes`), removes the worktree, and releases its port claim. Refuses a dirty worktree without `--force`. |
| `path` | Prints an instance's worktree directory, for `cd "$(treehouse path foo)"`. |
| `up` / `down` / `restart` / `status` | Context-aware compose wrappers: instance-scoped inside an instance, plain passthrough in a main repository. |
| `compose` | Raw `docker compose` passthrough, with `--env-file`/`--project-name` injected inside an instance. |
| `exec` | Sugar for `treehouse compose exec <service> <command...>`. |
| `provision` | Re-runs symlink/copy/hook provisioning; `--retry` resumes after a failed hook. |

`gc`, `prune`, `init`, `doctor`, and `switch` are not implemented yet (wave 3).
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
