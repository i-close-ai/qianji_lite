# Adding providers

Two layers: change official Pi first, then `qianji init`. Do not add custom scripts under `~/.pi`.

## 1. Official Pi: `~/.pi/agent/models.json`

Docs: [models.md](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/models.md)

Qianji never reads this file, nor `auth.json` or `settings.json`. Put provider
credentials only in Pi, using Pi's own mechanisms (environment variable
reference, `/login`, or a secret helper).

Example **shape** (replace values; do not commit real keys):

```json
"my-openai": {
  "baseUrl": "https://api.example.com/v1",
  "api": "openai-completions",
  "apiKey": "$MY_PROVIDER_KEY",
  "compat": { "supportsDeveloperRole": false },
  "models": [{ "id": "my-model", "reasoning": true }]
}
```

Other official Pi forms:

- `"apiKey": "$MY_PROVIDER_KEY"` — export the variable in the shell first
- omit `apiKey`, run `/login` in Pi, stored in `~/.pi/agent/auth.json`
- `"apiKey": "!op read 'op://...'"` — Pi command substitution, not a Qianji script

Claude-compatible: `"api": "anthropic-messages"`. Responses: `"api": "openai-responses"`.

Keep Pi credential files mode `600`. Official OpenAI/Anthropic can use `/login`
or environment variables and need not appear in `models.json`. Trust
`pi --list-models`.

## 2. Qianji: import

```bash
qianji init      # merge current `pi --list-models`
qianji reinit    # force merge, keep existing weights
```

If `pi` is missing, re-run `tools/install.sh` or:

`npm install -g --ignore-scripts @earendil-works/pi-coding-agent`

Init asks Pi for currently usable models (custom + authenticated official
providers) and merges into `~/.qianji/config.toml` (old weights kept, new
models `weight = 1`).

## 3. Call it

```bash
qianji run --model my-openai/my-model --effort high --prompt "..."
qianji run --route my-openai-my-model --prompt "..."
```
