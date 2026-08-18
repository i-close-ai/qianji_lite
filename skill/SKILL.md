---
name: qianji
description: >-
  Routes selected coding tasks through Qianji to Pi-backed models. Use when
  the user asks to 用 qianji, 使用qianji, 使用qianji强模型, 使用qianji最强模型,
  千机路由, /qianji, or to dispatch work by importance, cost, provider, model,
  or effort/effect. Host-agnostic: any agent that can run a local CLI can
  use this skill.
---

# Qianji（千机）

Qianji is a **model router**, not an access layer. Credentials and protocols
stay in official Pi (`~/.pi/agent/models.json`, or `/login` → `auth.json`).
Routing policy lives in `~/.qianji/config.toml`.

This skill is **host-agnostic**. Cursor, Codex, Claude Code, and other agents
invoke the same `qianji` binary through whatever shell tool that host provides.
Do not require Cursor-only, Codex-only, or Claude-only wrappers.

Do not copy API keys into Qianji config, prompts, or logs. Do not add custom
key scripts under `~/.pi`. Execution backend is `pi --print --no-session`.

`qianji init` checks that `pi` is installed. Catalog comes from
`pi --list-models` (custom + authenticated official providers), not from
scanning Pi config files. The first Qianji command each local day re-checks
that catalog's sha256 and merges on change. Force refresh: `qianji reinit`.

## When to use

- User says 「用 qianji」「使用qianji」「使用qianji强模型」「使用qianji最强模型」「千机路由」 or `/qianji`
- Work should be dispatched by importance / cost / provider / health

Do not send architecture trade-offs, security-critical edits, or unauthorized
destructive operations to the ordinary pool.

## 口令 → 档位

| User says | `--tier` | Actual model |
|---|---|---|
| 使用qianji / 用 qianji / 千机路由 | omit (ordinary pool) | Pi catalog, weighted random + cache affinity |
| 使用qianji强模型 | `--tier strong` | pinned route in `~/.qianji/config.toml` (`[tiers.strong]`) |
| 使用qianji最强模型 | `--tier strongest` | pinned route in `~/.qianji/config.toml` (`[tiers.strongest]`) |

Legacy aliases: `--tier main` = strong, `--tier important` = strongest.
Chinese `强模型` / `最强模型` are accepted.

档位 is Qianji-only. Pi only has model + thinking
(`off|minimal|low|medium|high|xhigh|max`). After `qianji init`, inspect
`qianji status` to see which models were pinned.

## How to run

Use the host's shell/terminal tool. One `run` = one work unit. Wait at least
10 minutes for a run to finish.

Ordinary pool. **`--affinity-key` must be the user's original request text**:

```bash
qianji run --workdir "$PWD" --affinity-key "<original user request>" --prompt "<ordinary task>"
```

Strong / strongest:

```bash
qianji run --tier strong --workdir "$PWD" --prompt "<task>"
qianji run --tier strongest --workdir "$PWD" --prompt "<task>"
```

Named model (`provider/model`):

```bash
qianji run --model provider-name/model-id --effort high --workdir "$PWD" --prompt "<task>"
```

`--effect` is an alias of `--effort`. Prompt may also come from `--prompt-file` or stdin.

```bash
qianji init
qianji reinit
qianji status
qianji choose --json
qianji simulate --count 2000
qianji doctor
qianji skill install
```

Exit codes: `0` success; `1` attempts exhausted; `2` consecutive timeouts; `75` all circuits open.

## Adding models

1. Configure Pi (official docs). Never put keys in Qianji files.
2. Confirm `pi --list-models` lists the model.
3. `qianji init` or `qianji reinit`. New routes start at `weight = 1`.

See [providers.md](references/providers.md) and [routing.md](references/routing.md).
Host install paths: [hosts.md](references/hosts.md).

## After it returns

Report route id, provider, model, effort, `via=affinity|weighted_random`, and
exit code. If Pi changed files, check `git status` / `git diff`.
