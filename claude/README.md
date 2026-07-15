# Claude Code integration

Two pieces, independent of each other — wire up either or both:

- **`session-end-hook.sh`** — a SessionEnd hook that runs `treehouse down`
  in whatever instance a Claude Code session was working in, so containers
  don't keep running once the session ends. It never removes anything and
  never fails the session teardown itself (see the script's own header
  comment for the exact contract).
- **`skill/`** — a Claude Code skill (`SKILL.md`) that teaches an agent how
  to use an already-adopted treehouse repository: creating/reusing an
  instance, running commands inside it, and — importantly — never running
  `treehouse remove` without asking first.

Both assume the *target* repository (the one an agent will be working in)
has already adopted treehouse — see [`../docs/adoption.md`](../docs/adoption.md).
This `claude/` directory is treehouse's own integration source; you copy or
link its contents into each repository where you want Claude Code to use
them.

## Wiring up the SessionEnd hook

Add a `SessionEnd` entry to that repository's `.claude/settings.json` (or
`.claude/settings.local.json` if you'd rather keep it out of version
control) pointing at the script, either copied in directly or referenced
via `$CLAUDE_PROJECT_DIR` if you keep treehouse's own repository checked
out alongside:

```json
{
  "hooks": {
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/claude/session-end-hook.sh"
          }
        ]
      }
    ]
  }
}
```

Omitting `matcher` (or setting it to `"*"`/`""`) matches every SessionEnd
reason (`clear`, `resume`, `logout`, `prompt_input_exit`,
`bypass_permissions_disabled`, `other`) — you want that here, since the
hook is a no-op unless the session was actually inside a treehouse instance
regardless of why it ended.

`$CLAUDE_PROJECT_DIR` is set by Claude Code to the project root for every
hook invocation, so the command above works as long as `claude/` (this
directory, copied from treehouse's own repository) exists at that path.
Most repositories will instead vendor just the one script:

```
mkdir -p .claude/hooks
cp path/to/treehouse/claude/session-end-hook.sh .claude/hooks/
chmod +x .claude/hooks/session-end-hook.sh
```

and point `command` at `"$CLAUDE_PROJECT_DIR/.claude/hooks/session-end-hook.sh"`
instead. Either layout works — the script itself has no dependency on
where it lives, only on `$CLAUDE_PROJECT_DIR` (or, failing that, its own
`cwd`) and `.treehouse.env` being findable from there.

## Wiring up the skill

Project-scoped Claude Code skills live at `.claude/skills/<name>/SKILL.md`
in the repository they apply to. Either copy or symlink treehouse's
`claude/skill/` directory there:

```
mkdir -p .claude/skills
cp -R path/to/treehouse/claude/skill .claude/skills/treehouse
```

or, to track upstream changes instead of a point-in-time copy:

```
ln -s path/to/treehouse/claude/skill .claude/skills/treehouse
```

Commit whichever you choose so every contributor's Claude Code picks it up
automatically — no further configuration needed; skills under
`.claude/skills/` are discovered without a `settings.json` entry.

## Verifying it worked

- Hook: start a Claude Code session inside a treehouse instance's worktree,
  end the session, and confirm `treehouse list` (or `docker ps`) shows that
  instance's containers stopped.
- Skill: ask Claude, from inside an adopted repository, to "set up an
  isolated environment for branch X" — it should reach for `treehouse
  create X --json` rather than working directly in the main checkout.
