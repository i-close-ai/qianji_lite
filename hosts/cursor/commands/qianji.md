---
name: qianji
description: Route the current task through Qianji to a Pi-backed model
---

Follow the `qianji` skill (host-agnostic CLI). Phrase mapping:

- 「使用qianji」→ ordinary pool (no `--tier`)
- 「使用qianji强模型」→ `--tier strong`
- 「使用qianji最强模型」→ `--tier strongest`

If the user named a model or effort/effect, pass `--model` / `--effort`.

For ordinary-pool runs, pass `--affinity-key` with the **original user request**.

Run via Shell: `qianji run --workdir "$PWD"`. `block_until_ms` at least `600000`.
Put the enhanced prompt in `--prompt` or `--prompt-file`. After it finishes,
report `via=` and verify any file changes.
