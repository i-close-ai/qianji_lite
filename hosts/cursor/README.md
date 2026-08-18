# Qianji — Cursor host adapter

Optional. The product is the Go CLI plus the host-agnostic skill
(`skill/SKILL.md`). After `qianji skill install` (or `tools/install.sh`),
Cursor discovers the skill from `~/.agents/skills/qianji` or
`~/.cursor/skills/qianji`.

This directory only adds a Cursor slash command. Set `block_until_ms` to at
least `--timeout × 1000` (ordinary pool: 2×). Skill defaults: 900s / 1800s /
2400s; large or 强/最强 tasks use 3600s (`block_until_ms` 3600000).
