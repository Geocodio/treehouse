# Port model

Treehouse's core promise is that two instances — anywhere on the machine,
in any repository, created by any developer or agent — never end up
publishing docker-compose ports that collide. This document explains how,
and how to configure or troubleshoot it. The authoritative behavioral
contract lives in [`DESIGN.md`](DESIGN.md)'s "Port registry" section; this
is the same story told for a reader who wants the reasoning, not just the
rules.

## Why blocks, not individual ports

An instance doesn't request "a free port" one at a time for each service it
publishes — it claims one **contiguous block** of `ports.block_size`
consecutive ports (32 by default) up front, and every port that instance's
compose stack publishes lives somewhere inside that one block (see
[`manifest.md`](manifest.md#ports) for how a manifest maps environment
variable names to indices within the block).

That's what makes cross-instance collisions **structurally impossible**
rather than merely unlikely: allocation only ever hands out non-overlapping
blocks (see "Allocation" below), so there is no cleverness elsewhere in the
codebase that needs to get port math right — every consumer of a port
number just reads it out of `.treehouse.env`, already resolved.

## The registry: one file, one machine

Every treehouse-managed repository on the machine shares a single
machine-global JSON file:

```
${XDG_STATE_HOME:-~/.local/state}/treehouse/registry.json
```

guarded by an exclusive `flock` on a sibling `registry.lock` for every
read-modify-write — allocation, release, and GC all take this same lock, so
two `treehouse create` invocations racing (in two repositories, or two
terminals in the same one) can never both read the registry in the state
"before either claimed a block" and both allocate the same one. The
concurrent-allocation property is exercised directly by a test that fires
20 goroutines at `Store.Claim` simultaneously and asserts every resulting
block is disjoint from every other.

Each entry (a *claim*) records which project and instance owns a block,
the repository and worktree paths (used for staleness checks — see below),
and the block's start and size:

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

## Allocation

`treehouse create` claims a block before it touches git or the filesystem
at all — if allocation fails (the pool is exhausted), nothing else has
happened yet, so there's nothing to roll back. The algorithm, run under the
lock:

1. **Garbage-collect stale claims first.** A claim whose `worktree` no
   longer exists on disk (removed by hand outside `treehouse
   remove`/`prune`, or the machine was reimaged) is dropped before scanning
   for a free block, so its block becomes available again immediately
   rather than requiring a separate `treehouse gc` first.
2. **Scan the pool** (`pool_start` to `pool_end` inclusive, default
   4000–9999, configurable — see [`manifest.md`](manifest.md#machine-config-configtreehouseconfigyml))
   in `block_size` steps, and take the **first** block that:
   - overlaps no remaining claim, **and**
   - has no port currently bound, per a single `lsof -nP -iTCP
     -sTCP:LISTEN` invocation parsed once (never one `lsof` call per
     candidate port — that would make a large pool scan slow, and a
     single invocation gives an atomic-enough snapshot of what's
     listening).
3. **Write the claim and release the lock.**

If no block in the pool satisfies both conditions, `Claim` fails with
`ErrPoolExhausted`, which `treehouse create` maps to exit code `2`. Widen
`pool_start`/`pool_end` in the machine config if you hit this routinely.

A request for a project/instance pair that **already has a claim** is
idempotent — `Claim` returns the existing block unchanged rather than
erroring or allocating a second one, matching `create`'s own
idempotent-per-branch contract.

## Why `lsof`, and what happens without it

The registry alone only knows about ports *it* has claimed — not every
port bound on the machine (a developer's own `docker run -p 5432:5432`,
a non-treehouse service, etc.). Checking real listening sockets via `lsof`
catches those cases too, so a freshly allocated block doesn't immediately
collide with something the registry never knew about.

If `lsof` isn't installed (or fails for any other reason), allocation
degrades to **registry-only** overlap checking rather than failing --
Claim returns a warning (`create`/`list` print it to stderr) instead of an
error. This is a deliberate availability trade-off: a missing `lsof` binary
shouldn't be able to block every instance creation on the machine, but it
does mean allocation can no longer see ports something *outside*
treehouse's own bookkeeping is using.

The same `lsof` snapshot also backs `up`'s own port re-verification (see
below) and, best-effort, names the process squatting on a conflicting port
when `up` refuses to start.

## `up` re-verifies before starting

Bringing a stack up (`treehouse up`, or the equivalent step inside `create`
and `provision`) doesn't just trust the registry's claim — it re-checks the
instance's actual block against currently listening sockets **every time**,
not only when the project has zero running containers. This matters
because the registry can only prevent collisions between *treehouse's own*
claims; a raw `docker run` or an unrelated process binding a port inside an
already-claimed block would otherwise go unnoticed until `docker compose
up` itself failed with a much less specific error.

Ports the instance's own containers already publish are excluded from the
comparison first (queried via `docker ps` against the compose project
label), so a partially-up stack — e.g. one service still starting — is
never mistaken for a conflict with itself. A genuine conflict fails with an
error naming the specific port, its manifest environment variable name (if
recognized), and, when `lsof` can identify it, the squatting process.

## Reaping: gc and prune

Two related but distinct reclaiming mechanisms, both documented in full in
[`DESIGN.md`](DESIGN.md#command-behavior):

- **`treehouse gc`** (and the opportunistic pass `create`/`list` trigger at
  most once an hour) stops idle instances' containers and reaps stale
  registry claims — worktrees that no longer exist on disk. It never
  removes a worktree itself, and it never touches a claim whose worktree
  still exists, no matter how idle.
- **`treehouse prune`** goes further: it actually removes idle instances
  (worktree, stack, and claim together) via the same code path as
  `treehouse remove`, after confirming each one individually — but a dirty
  worktree is always treated as in-use and skipped, at any age.

Either way, a claim's block only ever becomes reusable once its worktree is
gone — reaping a stale claim and allocating a fresh one into the same
numeric range are two separate, independently safe operations, both
guarded by the same lock.

## Verifying by hand

To see what treehouse itself sees:

```
lsof -nP -iTCP -sTCP:LISTEN   # every listening TCP port on the machine
treehouse list --all-repos    # every registry claim, across every repository
treehouse doctor              # flags stale claims and orphaned instances
```

A claim reported `"stale": true` by `list --all-repos` means its worktree
is gone but the claim (and its block) hasn't been reaped yet — run
`treehouse gc` to reclaim it, or wait for the next opportunistic pass.
