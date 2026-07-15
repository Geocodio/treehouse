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
treehouse version
```

See [`docs/DESIGN.md`](docs/DESIGN.md) for the full command reference and
implementation contract; only `treehouse version` ships in this wave.

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
