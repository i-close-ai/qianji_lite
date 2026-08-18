# Qianji Lite（千机）

Pi 后端的**通用模型路由器**。任意能跑本地 CLI 的 agent（Cursor、Codex、Claude Code 等）共用同一条 skill。

Qianji 只负责选路。供应商、协议和凭据留在官方 [Pi](https://github.com/earendil-works/pi)。本仓库不含 API key，安装脚本也不会写入 key。

## 一键安装

```sh
sh -c "$(curl -fsSL https://raw.githubusercontent.com/i-close-ai/qianji_lite/main/tools/install.sh)"
```

或：

```sh
sh -c "$(wget -qO- https://raw.githubusercontent.com/i-close-ai/qianji_lite/main/tools/install.sh)"
```

脚本会：

1. 检查 / 安装 **Go 1.22+** 和 **Node.js**（macOS 上若有 Homebrew 会代装）
2. 安装官方 Pi CLI：`npm install -g --ignore-scripts @earendil-works/pi-coding-agent`
3. 克隆本仓库到 `~/.qianji-lite`（若已在仓库里执行 `sh tools/install.sh`，则用当前目录）
4. 编译 `qianji` 到 `~/.local/bin`
5. 安装通用 skill（`~/.agents/skills/qianji` 等）
6. 尝试 `qianji init`（需要 Pi 里已经能 `pi --list-models`）

可选环境变量：`REPO`、`REMOTE`、`BRANCH`、`QIANJI_DIR`、`PREFIX`、`SKIP_GO=1`、`SKIP_NODE=1`、`SKIP_PI=1`、`SKIP_INIT=1`、`GOPROXY`。

卸载：

```sh
sh tools/uninstall.sh
```

不会删除 `~/.qianji/config.toml` 或 `~/.pi` 凭据。

## 从源码安装

```sh
git clone https://github.com/i-close-ai/qianji_lite.git
cd qianji_lite
sh tools/install.sh
```

或：

```sh
go test ./...
go build -o ~/.local/bin/qianji ./cmd/qianji
qianji skill install
qianji init
```

国内若 `go` 模块下载超时，可：

```sh
GOPROXY=https://goproxy.cn,direct sh tools/install.sh
```

## 架构

```
Agent (Cursor / Codex / Claude / …)
        │  调用 qianji CLI
        ▼
   Qianji Lite          ~/.qianji/config.toml   权重、档位、熔断
        │  exec
        ▼
   官方 Pi              ~/.pi/agent/            模型与凭据
```

- **Pi**：`pi --list-models`、`pi --provider --model --print --no-session`
- **Qianji**：加权随机 + 缓存亲和 + 熔断；「强 / 最强」是 Qianji 档位，映射到 Pi 的 model + thinking
- Qianji **运行时不打开** `models.json` / `auth.json`

## 口令

| 你说 | 行为 |
|---|---|
| 使用qianji / 用 qianji / 千机路由 | 普通池：加权随机 + 缓存亲和 |
| 使用qianji强模型 | `--tier strong` |
| 使用qianji最强模型 | `--tier strongest` |

档位对应的具体模型写在 `~/.qianji/config.toml` 的 `[tiers.strong]` / `[tiers.strongest]`，由 `qianji init` 按当前 Pi 目录生成，可自行改。

## 命令

```bash
qianji run --workdir "$PWD" --affinity-key "原始用户请求" --prompt "..."
qianji run --tier strong --workdir "$PWD" --prompt "..."
qianji status
qianji doctor
qianji init
qianji reinit
```

普通池必须把**用户原文**传给 `--affinity-key`。一次 `run` 只做一个工作单元。

退出码：`0` 成功；`1` 尝试耗尽；`2` 连续超时；`75` 全部熔断。

## 配置 Pi（凭据只放这里）

按 [Pi models.md](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/models.md) 配置自定义供应商，或对官方 OpenAI / Anthropic 使用 `/login` / 环境变量。

配置完后：

```bash
pi --list-models
qianji init          # 新模型 weight = 1，旧权重保留
```

含凭据的 Pi 文件请 `chmod 600`。**不要**把 key 写进本仓库、Issue、Qianji 的 `config.toml` 或 skill。

## 通用 skill

`qianji skill install` 解包到 `~/.qianji/skill`，并链到：

- `~/.agents/skills/qianji`
- 若本机已有对应目录：`~/.cursor/skills`、`~/.claude/skills`、`~/.codex/skills`

Agent 约定：用宿主自己的 shell 跑 `qianji`，不要依赖某个 IDE 的私有包装。

## 安全

- 本仓库与安装脚本**不包含、不采集、不打印** API key
- 路由状态在 `~/.qianji/state.json`（熔断与亲和，不是密钥）
- 发现密钥被提交：立刻轮换该 key，不要只 `git rm`

## 开发

```sh
make test
make build
```

需要 Go 1.22+。

## License

[Apache License 2.0](LICENSE)
