# Manifest reference

This is the field-by-field reference for `.treehouse.yml`, the generated
`.treehouse.env` instance file, and the machine-local `config.yml`. For the
behavioral contract these files support (the state machine, port
allocation, gc, etc.), see [`DESIGN.md`](DESIGN.md); this document only
describes the file formats themselves.

## `.treehouse.yml`

Lives at the adopting repository's root and is committed to version
control. It is always read from **the invoking checkout's own worktree
root**, never from the repository's main worktree — so a branch's own copy
of `.treehouse.yml` is what governs commands run from that branch's
worktree, even while the main checkout is on a different branch that
hasn't merged the manifest yet (the ordinary state of a repository
mid-adoption). See [`DESIGN.md`](DESIGN.md)'s "Manifest resolution" note
for the full rule, including what stays anchored to the main repository
instead (instance discovery, and `provision`'s symlink/copy/copy_files
sources).

```yaml
version: 1
project: myapp

ports:
  block_size: 32
  env:
    HTTP_PORT: 0
    DATABASE_PORT: 1
    MAIL_UI_PORT: 2
  aliases:
    WEB_PORT: HTTP_PORT

provision:
  symlink:
    - data/shared
    - path: dev-certs
      optional: true
  copy:
    - path: vendor
      reconcile:
        lockfile: composer.lock
        command: composer install
    - path: node_modules
      reconcile:
        lockfile: package-lock.json
        command: npm install
    - public/build
  copy_files:
    - .env.docker
  hook: scripts/treehouse-post-create.sh

urls:
  - name: App
    url: "http://localhost:{HTTP_PORT}"

location: ~/treehouses
```

Unknown **top-level** keys are a fatal error (typo protection: a misspelled
`prject:` fails loudly rather than being silently ignored). Unknown keys
nested inside a known section (e.g. an unrecognized key under `ports:` or
inside a `provision.copy` entry) are a warning instead — printed to stderr,
never blocking the command — so a manifest written against a newer
treehouse version still loads on an older one.

### `version` (required)

Integer, currently only `1`. A manifest declaring a version higher than the
running treehouse binary understands is rejected outright (upgrade
treehouse) rather than partially parsed, since a newer schema version may
have changed what an existing key means.

### `project` (required)

The compose project-name prefix: every instance's docker-compose project
name is `<project>-<instance>`. Must match `[a-z0-9][a-z0-9_-]*` — lowercase
letters, digits, underscore, hyphen, and it cannot start with an underscore
or hyphen.

### `ports`

All of `ports` is optional; a manifest with no `ports` key at all still
works if the project publishes no ports (rare) or if `ports.block_size`'s
default is acceptable with an empty `env` map.

- **`block_size`** (default `32`): the number of consecutive host ports
  reserved per instance. Set this to comfortably more than the number of
  ports your compose file actually publishes, so future services fit
  without an instance's block needing to move.
- **`env`**: a mapping of environment variable name to that port's index
  within the block (`0` to `block_size - 1`). Every index must be unique
  and less than `block_size`. The absolute host port for a variable is
  `block_start + index`, where `block_start` is whatever the registry
  allocated this instance (see [`ports.md`](ports.md)). Variable names must
  be valid POSIX environment variable identifiers.
- **`aliases`**: a mapping of an *alias* environment variable name to the
  *canonical* `ports.env` variable name it should mirror. Some
  applications read the same published port under more than one
  environment variable name (a legacy config var alongside a newer one,
  for instance) — rather than wasting a second block index on a port
  that's really the same value twice, an alias is written into
  `.treehouse.env` with exactly the same value as its canonical variable
  and consumes no index of its own. Validation requires every alias's
  canonical variable to actually exist in `ports.env`, and an alias name
  must not collide with a `ports.env` key (the two namespaces are
  disjoint).

### `provision`

Everything under `provision` is optional and runs in this fixed order:
`symlink`, then `copy` (each entry, including its `reconcile` check), then
`copy_files`, then `hook` — matching the order a repository's own
onboarding steps usually run in (shared data first, then per-instance
mutable dependencies, then small config files, then anything scripted).

- **`symlink`**: a list of paths (relative to the repository root) to
  symlink into the instance, pointing at the *main* repository's copy.
  Use this for large, genuinely shared, read-mostly data that every
  instance should see identically (seed data, shared uploads in
  development, etc.) — never for anything an instance needs to mutate
  independently, since all instances share the same underlying files.
  Each entry takes one of two forms:
  - a plain string (`data/shared`): a **required** shared path.
  - a mapping with `path` and `optional: true`:
    ```yaml
    - path: dev-certs
      optional: true
    ```
  If the destination is already a symlink, it's replaced; if it's an
  empty real directory, that directory is removed first; a *non-empty*
  real directory refuses with an error rather than silently discarding its
  contents. A source that doesn't exist anywhere in the main repository is
  **a hard failure by default** (never a warning): unlike `copy`/`copy_files`
  below, `symlink` is normally for data an instance never generates on its
  own, so a missing source is usually a real misconfiguration, not a routine
  "not built yet" state. An entry flagged `optional: true` opts out of that
  strictness — its missing source is skipped with a warning ("skipping
  dev-certs: not present in main repository") and provisioning continues,
  for a symlink target the main repository may legitimately not have
  generated yet (a dev-cert bundle, a build-artifact directory). A present
  source is symlinked identically whether or not the entry is optional.
- **`copy`**: a list of paths to copy-on-write clone into the instance —
  each instance gets its own independent, mutable copy, cheaply (see
  [`ports.md`](ports.md)'s sibling section in `DESIGN.md` on the
  copy-on-write strategy: `cp -Rc`/`cp -a --reflink=auto`, never a Go
  file-walk). Two forms:
  - a plain string (`public/build`): copied once, never reconciled again.
  - a mapping with `path` and an optional `reconcile` block:
    ```yaml
    - path: vendor
      reconcile:
        lockfile: composer.lock
        command: composer install
    ```
    `reconcile.lockfile` is a path (relative to the repository root) whose
    sha256 is recorded at clone time; `reconcile.command` re-runs only when
    the *main repository's* copy of that lockfile has drifted since the
    instance's directory was last cloned or reconciled — never
    unconditionally on every `provision`. Both `lockfile` and `command` are
    required together. See the [adoption guide](adoption.md) for the
    reconcile command's idempotency contract (e.g. always `npm install`,
    never `npm ci`, which would delete the cloned directory).
  Copying is idempotent: an existing destination directory is left alone,
  so re-running provisioning (`provision --retry`) never clobbers a
  directory that was already cloned.

  A source that doesn't exist anywhere in the main repository (a generated
  build-artifact directory nobody has built there yet — `vendor/`,
  `node_modules/`, `public/build`) is a warning, not a failure: that one
  entry is skipped ("skipping vendor: not present in main repository") and
  `create`/`provision` continues. If that entry also declares `reconcile`,
  its `reconcile.command` still runs — immediately and unconditionally,
  since with no directory cloned at all there's no lockfile hash yet for
  the usual drift check to compare against; only once that first run
  completes does the lockfile's hash get recorded, so later
  `provision`/`provision --retry` calls go back to the ordinary
  drift-only behavior.
- **`copy_files`**: a list of individual files (not directories) plainly
  copied — never CoW-cloned, since these are expected to be small
  (`.env.docker`, per-instance override files). Also idempotent: an
  existing destination file is left untouched, so a developer's local
  edits to a previously copied file survive `provision --retry`. Like
  `copy`, a missing source is a warning and a skip, never a failure — and
  since `copy_files` has no `reconcile` concept, there's nothing further
  to force.
- **`hook`**: a path (relative to the repository root) to a script run
  once the compose stack is up, with the instance's full `.treehouse.env`
  contents exported into its environment plus `TREEHOUSE_INSTANCE_PATH`.
  See the [adoption guide](adoption.md) for the hook's idempotency
  contract and a database-readiness wait example. A missing/absent `hook`
  key is not an error — it's simply skipped.

`hook`, every `symlink` entry, every `copy`/`copy_files` path, must be
relative and must not escape the repository root (no `..` segments) —
validation rejects an absolute path or a `../`-prefixed one outright.

### `urls`

A list of human-facing summary lines printed after `create`/`provision`
finish (and returned in `create --json`'s `"urls"` field), each with a
`name` and a `url` template. Every `{ENV_VAR}` placeholder in `url` is
substituted with that instance's actual port number for the matching
`ports.env` (or `ports.aliases`) variable:

```yaml
urls:
  - name: App
    url: "http://localhost:{HTTP_PORT}"
  - name: Mail UI
    url: "http://localhost:{MAIL_UI_PORT}"
```

Both `name` and `url` are required on every entry.

### `location`

Optional override of where this project's instances are created, taking
precedence over the machine config's `instances_home` (but not over
anything — there is no higher-precedence override). Accepts `~`-relative
paths (`~/treehouses`), expanded against the current user's home directory.
Absolute paths are also fine. Unlike `hook`/`symlink`/`copy`/`copy_files`,
`location` is not required to be relative — it names a directory outside
the repository by design.

## Generated: `.treehouse.env`

Written atomically (temp file + rename) at the root of every instance's
worktree — never committed (add it to the adopting repository's
`.gitignore`). It serves two purposes at once: it's a valid docker-compose
`--env-file`, and it's the marker file treehouse and adopting repo tooling
(`Taskfile`/`Makefile`, the Claude Code hook) use to detect "am I inside a
treehouse instance right now".

```
# Managed by treehouse — do not edit. Regenerate with: treehouse provision
COMPOSE_PROJECT_NAME=myapp-my-feature
TREEHOUSE_PROJECT=myapp
TREEHOUSE_INSTANCE=my-feature
TREEHOUSE_BRANCH=feature/my-feature
TREEHOUSE_MAIN_REPOSITORY=/home/user/projects/myapp
TREEHOUSE_BLOCK=4640
TREEHOUSE_STATE=provisioned
TREEHOUSE_LOCK_HASH_composer.lock=<sha256 of the main repo's lockfile at clone/reconcile time>
HTTP_PORT=4640
DATABASE_PORT=4641
MAIL_UI_PORT=4642
```

| Key | Meaning |
|---|---|
| `COMPOSE_PROJECT_NAME` | `<project>-<instance>`; the docker-compose project name, also what container queries filter on via the `com.docker.compose.project` label. |
| `TREEHOUSE_PROJECT` | The manifest's `project` name. |
| `TREEHOUSE_INSTANCE` | This instance's name (the branch slug, possibly auto-suffixed on collision). |
| `TREEHOUSE_BRANCH` | The full branch name this instance tracks (e.g. `feature/my-feature`), as opposed to `TREEHOUSE_INSTANCE` which is derived from just its last path segment. |
| `TREEHOUSE_MAIN_REPOSITORY` | Absolute path to the repository's main worktree (never a linked/instance worktree). |
| `TREEHOUSE_BLOCK` | The first port of this instance's claimed block. |
| `TREEHOUSE_STATE` | One of `worktree-created`, `deps-cloned`, `stack-up`, `provisioned`, `provision-failed`. |
| `TREEHOUSE_LOCK_HASH_<lockfile>` | One line per `provision.copy` entry with a `reconcile` step, e.g. `TREEHOUSE_LOCK_HASH_composer.lock`, `TREEHOUSE_LOCK_HASH_package-lock.json`. The recorded sha256 hex digest `provision`/`provision --retry` diffs the current lockfile against to decide whether to re-run `reconcile.command`. |
| every `ports.env`/`ports.aliases` name | The instance's absolute port number for that variable (`block_start` + the manifest's configured index; an alias mirrors its canonical variable's value exactly). |

Lock-hash and port lines are always emitted sorted by key, so the file is
byte-for-byte deterministic across runs — useful if you ever diff it. Also
exported into the post-create hook's process environment (via `treehouse
provision`/`create`), alongside one extra variable the file itself doesn't
contain: `TREEHOUSE_INSTANCE_PATH` (the instance's absolute worktree path).

## Machine config: `~/.config/treehouse/config.yml`

Machine-local (never committed, doesn't belong to any one repository),
resolved from `$XDG_CONFIG_HOME/treehouse/config.yml`, defaulting to
`~/.config/treehouse/config.yml`. Every key is optional; a missing file, or
a file missing some keys, simply yields defaults — this is not an error.

```yaml
instances_home: ~/treehouses
pool_start: 4000
pool_end: 9999
```

| Key | Default | Meaning |
|---|---|---|
| `instances_home` | `~/treehouses` | Where instance worktrees are created, as `<instances_home>/<project>/<instance>`, unless a manifest's `location` overrides it for that one project. Accepts a `~`-relative path. |
| `pool_start` | `4000` | Inclusive lower bound of the machine-wide port pool the registry scans for a free block. |
| `pool_end` | `9999` | Inclusive upper bound of the same pool. |

Widen `pool_start`/`pool_end` if you routinely run enough concurrent
instances across enough repositories that the default ~187 blocks of 32
(`(9999-4000+1)/32`) isn't enough headroom; see [`ports.md`](ports.md) for
the allocation algorithm this pool feeds.
