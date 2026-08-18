# Qianji Lite（千机）

Pi 后端的**模型路由器**。Cursor、Codex、Claude Code 等能跑本地 CLI 的 agent 共用同一条 skill、同一个 `qianji` 二进制。

Qianji 只选路。供应商、协议和 API key 留在官方 [Pi](https://github.com/earendil-works/pi)。本仓库和安装脚本不包含、不采集、不写入密钥。

```
宿主 agent  ──qianji CLI──►  Qianji Lite     ~/.qianji/config.toml
                                │               权重 / 档位 / 熔断
                                ▼
                             官方 Pi         ~/.pi/agent/
                                             模型与凭据
```

- 目录来自 `pi --list-models`，运行时**不读** `models.json` / `auth.json` / `settings.json`
- 执行：`pi --provider --model --print --no-session`，prompt 走 stdin
- 「强 / 最强」是 Qianji 档位，映射到 Pi 的 model + thinking

## 安装

需要 **Go 1.22+**、**Node.js**（用来装 Pi）、以及 PATH 里能找到 `~/.local/bin`。

**一键（会 clone 到 `~/.qianji-lite` 并编译）：**

```sh
sh -c "$(curl -fsSL https://raw.githubusercontent.com/i-close-ai/qianji_lite/main/tools/install.sh)"
```

已在本仓库里时，直接 `sh tools/install.sh`（用当前目录，不再 clone）。国内模块超时可加 `GOPROXY=https://goproxy.cn,direct`。

**已有源码、只编译：**

```sh
go test ./...
go build -o ~/.local/bin/qianji ./cmd/qianji
qianji skill install
qianji init
```

`qianji skill install` **不需要仓库地址**。skill 已 embed 进二进制，会解到 `~/.qianji/skill/`，并 symlink 到已存在的宿主目录（`~/.agents/skills`、`~/.cursor/skills`、`~/.claude/skills`、`~/.codex/skills`）。改过 `skill/SKILL.md` 后要重新 `go build` 再 `skill install` 才会生效。

装完后：

```sh
qianji doctor
qianji status
```

可选环境变量：`REPO`、`REMOTE`、`BRANCH`、`QIANJI_DIR`、`PREFIX`、`SKIP_GO=1`、`SKIP_NODE=1`、`SKIP_PI=1`、`SKIP_SKILL=1`、`SKIP_INIT=1`、`GOPROXY`。

## 配置只保留一份

| 路径 | 作用 |
|---|---|
| `~/.qianji/config.toml` | 唯一路由配置（权重、档位） |
| `~/.qianji/state.json` | 熔断与亲和状态，不是第二份配置 |
| `~/.qianji/skill/` | skill 正文；各 IDE 只是链接过来 |
| `~/.pi/agent/` | Pi 的模型与凭据，Qianji 不写这里 |

不要把 key 放进 `config.toml`、skill、Issue 或本仓库。Pi 凭据文件建议 `chmod 600`。

## 配置 Pi

按 [Pi models.md](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/models.md) 配自定义供应商，或对官方 OpenAI / Anthropic 用 `/login` / 环境变量。然后：

```bash
pi --list-models
qianji init          # 新模型 weight = 1，已有权重保留
```

`qianji init` / `reinit` 会丢掉 Pi 目录里已经不存在的路由。日常命令只做每日检查：已检查过则跳过；新模型并入，缺失的路由**先留着**。

## 口令

宿主 agent 读 skill，用自己的 shell 跑 `qianji`，不要为某个 IDE 再包一层。

| 你说 | 行为 |
|---|---|
| 使用qianji / 用 qianji / 千机路由 | 普通池：加权随机 + 缓存亲和 |
| 使用qianji强模型 | `--tier strong` → `config.toml` 的 `[tiers.strong]` |
| 使用qianji最强模型 | `--tier strongest` → `[tiers.strongest]` |

档位模型由 `qianji init` 按当前 Pi 目录生成，可改。普通池必须把**用户原文**传给 `--affinity-key`。一次 `run` 只做一个工作单元。

## 超时

未传 `--timeout` 时按档位自动选。大任务请显式加长。宿主等待（Cursor：`block_until_ms`）必须 **≥ 这次 timeout**；普通池可能超时后再试一跳，建议按 2 倍留。

| 场景 | `--timeout` | 宿主等待 |
|---|---|---|
| 普通池 | 900s（15m） | ≥ 1800000 |
| 强模型 | 1800s（30m） | ≥ 1800000 |
| 最强模型 | 2400s（40m） | ≥ 2400000 |
| 大任务（任意档） | 3600s（60m） | ≥ 3600000 |

多文件重构、大范围审查，或你说「大任务 / 不要超时」时用 3600。强/最强做非琐碎改动时，skill 要求直接用 3600。

## 命令

```bash
qianji run --timeout 900 --workdir "$PWD" --affinity-key "原始用户请求" --prompt "..."
qianji run --tier strong --timeout 1800 --workdir "$PWD" --prompt "..."
qianji run --tier strongest --timeout 2400 --workdir "$PWD" --prompt "..."
qianji run --tier strong --timeout 3600 --workdir "$PWD" --prompt "..."   # 大任务
qianji status
qianji doctor
qianji init
qianji reinit
qianji skill install
```

退出码：`0` 成功；`1` 尝试耗尽；`2` 连续超时；`75` 全部熔断。

普通池里供应商失败会熔断该 `provider:model`（2m → 5m → 20m → 1h）并换模型重试；**超时不熔断**，本轮排除后最多再试一跳。`--tier` 失败或超时则直接退出。某模型不想再用：把 `weight` 设为 `0`。

## 运行日志

每次 Pi 尝试追加一行到 `~/.qianji/logs/runs.jsonl`（不含 prompt 和 API key）。

- **成功**：短记录（ts、route、via、elapsed_ms）
- **超时 / 失败 / 全熔断**：详细记录（error_type、error、output_head、timeout_sec、prompt_bytes 等）
- **体积**：当前文件约 4MB 时轮转成 `runs.jsonl.1` … `.4`，大约最多 20MB

超时会计入 `state.json` 的 `timeouts`，**不熔断**。`qianji status` 会打印日志路径。

`--print` 下官方 Pi 没有命令级权限弹窗。不要默认给 Pi 加 `--approve`：那只表示信任项目目录里的 `.pi` 扩展/设置，不是「全部授权」。

## 卸载

```sh
sh tools/uninstall.sh
```

只删 `~/.local/bin/qianji` 和 skill 的 symlink。`~/.qianji`（路由配置）和 `~/.pi`（凭据）会留下。若曾用一键安装，可自行删除 `~/.qianji-lite`。

## 开发

```sh
make test
make build
```

## License

[Apache License 2.0](LICENSE)
