# Install the skill on any agent

The product is the `qianji` CLI. The skill is instructions that tell an agent
when and how to call that CLI. One skill tree, many hosts.

Canonical copy after `qianji skill install`:

- `~/.qianji/skill/` (extracted from the binary)
- `~/.agents/skills/qianji` → that directory (shared by Cursor, Codex, and others)

## Host discovery paths

`qianji skill install` symlinks the same folder into whichever of these exist
or can be created:

| Host | Path |
|---|---|
| Shared / Codex / Cursor | `~/.agents/skills/qianji` |
| Cursor (user) | `~/.cursor/skills/qianji` |
| Claude Code (user) | `~/.claude/skills/qianji` |
| Codex (user) | `~/.codex/skills/qianji` |

Project-local copies (optional, not installed by default):

- `.agents/skills/qianji`
- `.cursor/skills/qianji`
- `.claude/skills/qianji`
- `.codex/skills/qianji`

Any other agent: copy or symlink `~/.qianji/skill` into that product's skills
directory, or tell the agent to run `qianji` directly.

## Agent contract

1. Detect the user's 口令 (ordinary / 强模型 / 最强模型).
2. Run `qianji run ...` in a shell. Do not reimplement routing.
3. For ordinary pool, pass `--affinity-key` as the **original user text**.
4. Wait long enough (at least 10 minutes).
5. Report `via=`, provider, model, effort, exit code.
6. Never print API keys or copy them into Qianji files.

Cursor-only extras (slash command, `block_until_ms`) live in the optional
Cursor plugin under `hosts/cursor/`, not in this skill.
