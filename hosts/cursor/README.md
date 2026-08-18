# Qianji — Cursor host adapter

Optional. The product is the Go CLI plus the host-agnostic skill
(`skill/SKILL.md`). After `qianji skill install` (or `tools/install.sh`),
Cursor discovers the skill from `~/.agents/skills/qianji` or
`~/.cursor/skills/qianji`.

This directory only adds a Cursor slash command. Keep `block_until_ms` ≥ 600000
when the agent runs `qianji run` through Cursor's Shell tool.
