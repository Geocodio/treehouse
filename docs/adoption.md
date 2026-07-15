# Adoption guide

Step-by-step guide to adopting treehouse in an existing docker-compose
project. For the full field reference once you're editing `.treehouse.yml`
by hand, see [`manifest.md`](manifest.md); for the port allocation model,
see [`ports.md`](ports.md).

## 1. Install

See the [README](../README.md#install) for the current install options
(release binaries, `go install`, or build from source). Confirm it's on
your `PATH`:

```
treehouse version
```

## 2. Run `treehouse init`

From your repository's root, next to `docker-compose.yml`:

```
treehouse init
```

This reads your compose file's published ports and prints a proposed
`.treehouse.yml`, plus (for any hardcoded host port) the exact compose diff
needed to parameterize it. It **never edits `docker-compose.yml` itself** —
you apply the diff by hand, which is also your chance to sanity-check the
proposed environment variable names. Once you're happy with it:

```
treehouse init --write
```

writes `.treehouse.yml` (refusing to overwrite one that already exists).

## 3. Read `treehouse doctor` and fix the compose contract

```
treehouse doctor
```

reports `[PASS]`/`[WARN]`/`[FAIL]` lines and exits nonzero if anything
fails. Three checks against `docker-compose.yml` matter most for a
first-time adoption:

### Hardcoded published ports (fail)

```yaml
# Before — fails doctor: two instances can't both publish host port 8080.
ports:
  - "8080:80"
```

```yaml
# After — passes: the port comes from the instance's own claimed block,
# falling back to the literal default when the variable is unset (e.g. a
# plain `docker compose up` outside any treehouse instance).
ports:
  - "${HTTP_PORT:-8080}:80"
```

Every published port needs this `${VAR:-default}` treatment, one manifest
`ports.env` entry per variable. `treehouse init` proposes the variable
names and the diff for you; you still need to apply it.

### `container_name` (fail)

```yaml
# Before — fails doctor: two instances of the same service can't both be
# named "myapp-web".
container_name: myapp-web
```

Delete the line entirely. Docker compose's own project-name-based
container naming (`<project>-<service>-<n>`) already keeps every
instance's containers uniquely named — that's precisely what `--project-name
$COMPOSE_PROJECT_NAME` (injected automatically by `treehouse
compose`/`up`/etc.) buys you. A hardcoded `container_name` throws that
away.

### Absolute bind mounts (warn, not fail)

```yaml
# Warns: works, but only on a machine with this exact path.
volumes:
  - /Users/you/data:/data
```

```yaml
# Prefer a named volume (one per compose project, i.e. one per instance
# automatically)...
volumes:
  - data:/data
# ...or a path relative to the compose file, which resolves inside
# whichever worktree the instance's own docker compose invocation runs
# from:
volumes:
  - ./data:/data
```

This is a warning rather than a failure because an absolute path *can*
still work (if it happens to exist identically on every machine you run
instances on), but it's rarely what you actually want across multiple
worktrees — a named volume or a relative bind mount is scoped per-instance
automatically, matching how ports and container names already work.

## 4. Write the post-create hook

`provision.hook` (see [`manifest.md`](manifest.md#provision)) is optional
but is where most of the value of an isolated environment comes from:
seeding a database, running migrations, warming a cache, generating an
app-specific config file, whatever your project's own onboarding already
does by hand.

Two contract points that matter more here than in an ordinary setup script:

**The hook is idempotent.** It runs once after a fresh `create`, but also
every time `treehouse provision` (or `provision --retry`) re-runs — a
failed hook leaves the instance inspectable rather than rolling anything
back, and re-running provisioning must not double-seed a database or
error out because a migration already ran. Guard destructive or
one-time-only steps accordingly (`CREATE TABLE IF NOT EXISTS`-style
idioms, a migration tool that already tracks what it's applied, etc.).

**The stack is already up by the time the hook runs.** Per the state
machine in [`DESIGN.md`](DESIGN.md#instance-model), the hook only ever
fires once the compose stack has reached `stack-up` — never before —
specifically so it's safe to depend on database containers already
running. But "the container is running" and "the database inside it is
ready to accept connections" are different moments; a fresh postgres or
mariadb container can take a few seconds to finish initializing. Poll for
readiness rather than assuming the first connection attempt succeeds:

```sh
#!/usr/bin/env sh
set -eu

# Wait for postgres to accept connections before running migrations.
for attempt in $(seq 1 30); do
	if docker compose --project-name "$COMPOSE_PROJECT_NAME" exec -T db pg_isready -U app >/dev/null 2>&1; then
		break
	fi
	sleep 1
done

docker compose --project-name "$COMPOSE_PROJECT_NAME" exec -T web php artisan migrate --force
echo "treehouse: post-create hook finished"
```

or, for mariadb/mysql:

```sh
for attempt in $(seq 1 30); do
	if docker compose --project-name "$COMPOSE_PROJECT_NAME" exec -T db mariadb-admin ping -uroot >/dev/null 2>&1; then
		break
	fi
	sleep 1
done
```

The hook runs with the instance's full `.treehouse.env` already exported
into its environment (see [`manifest.md`](manifest.md#generated-treehouseenv)),
so `$COMPOSE_PROJECT_NAME` and every `ports.env` variable are available
without sourcing anything.

## 5. Dispatch between `docker compose` and `treehouse compose`

Adopting treehouse should never break plain `docker compose` in the main
checkout — treehouse is required only where an instance actually exists,
and only `treehouse create` produces one, so the dependency is
self-consistent and strictly opt-in. The recommended pattern detects which
one applies at the point your Taskfile or Makefile invokes compose, by
checking for the instance marker file:

**Taskfile.yml:**

```yaml
vars:
  COMPOSE:
    sh: test -f .treehouse.env && echo "treehouse compose" || echo "docker compose"

tasks:
  up:
    cmds:
      - "{{.COMPOSE}} up -d"
  test:
    cmds:
      - "{{.COMPOSE}} exec web pytest"
```

**Makefile** (the shell-variable equivalent, since make doesn't have
Taskfile's `sh:` var evaluation):

```makefile
COMPOSE := $(shell test -f .treehouse.env && echo "treehouse compose" || echo "docker compose")

.PHONY: up test
up:
	$(COMPOSE) up -d

test:
	$(COMPOSE) exec web pytest
```

Run from a main checkout, `$(COMPOSE)`/`{{.COMPOSE}}` resolves to plain
`docker compose` and nothing about your existing workflow changes. Run
from inside a treehouse instance, it resolves to `treehouse compose`,
which injects that instance's `--env-file`/`--project-name` automatically
— so the *same* Taskfile/Makefile targets work correctly in both places
without an `if` in every single task.

## 6. Ignore the generated env file

`.treehouse.env` is generated per instance and is meaningless outside the
worktree it was written in — add it to `.gitignore`:

```
echo '.treehouse.env' >> .gitignore
```

(`.treehouse.yml`, by contrast, **is** committed — it's the shared manifest
every instance and the main checkout read.)

## 7. Optional: wire up Claude Code

If contributors (human or agent) use Claude Code against this repository,
treehouse ships a ready-made SessionEnd hook and skill — see
[`../claude/README.md`](../claude/README.md) for exact wiring instructions
(a `.claude/settings.json` snippet for the hook, and where to copy or
symlink the skill). This step is entirely optional and orthogonal to
everything above: treehouse itself has no dependency on Claude Code, and
the CLI works identically with or without it.

## Verifying the whole thing works

```
treehouse doctor                       # should be all PASS (or intentional WARN) by now
treehouse create feature/adoption-test --json
cd "$(treehouse path feature-adoption-test)"
treehouse status                       # containers running, state=provisioned
{{.COMPOSE}} test                      # or whatever your test task is now called
cd -
treehouse remove feature-adoption-test --force
```
