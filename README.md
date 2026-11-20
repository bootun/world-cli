<p align="center">
  <img src="assets/logo.svg" width="128" alt="World Machine" />
</p>

# World CLI (`world`) & Agent Skill

[World Machine](https://worldmachine.dev) 官方命令行工具 **`world`** 与配套 **`world-machine`** Agent Skill —— 为 AI 编程助手（Claude Code、Cursor、Trae、Antigravity 等）和终端工作流提供带时间线的结构化世界状态与原文记忆服务。

- **原话与结构化状态并存**：写入一句自然语言，既完整保留不可变原话，又自动抽取并演进结构化实体属性与生效时间区间。
- **任意历史时刻回溯**：通过 `--as-of` 指定历史日期，无损查询当时成立的世界状态切片。
- **开箱即用的终端与 Agent 体验**：自动检测本机 IANA 时区解析“昨天”“上个月”等相对时间，自动生成幂等键并安全重试瞬时网络抖动，支持浏览器一键授权登录。

---

## 最快上手：交给 AI 助手自动安装

如果你正在使用支持命令执行的 AI 编程助手，直接向它发送下面这句话即可自动完成 CLI 与 Skill 的安装和授权引导：

```text
帮我安装 World CLI：https://worldmachine.dev/cli-guide.md
```

---

## 手动安装

### 1. 安装 `world` CLI（Go 1.27+）

```sh
go install github.com/bootun/world-cli/cmd/world@latest

# 验证安装
world --version
```

或者克隆仓库在本地编译安装：

```sh
make build    # 编译到 ./bin/world
make install  # 安装到 $GOBIN/world
```

### 2. 安装配套 `world-machine` Agent Skill（推荐）

```sh
# 通过 skills CLI 一键安装
npx -y skills add bootun/world-cli --skill world-machine -y

# 或直接复制到本地 skills 目录
cp -r skills/world-machine ~/.agents/skills/world-machine
```

安装后，AI 编程助手无需等你显式下令“记住”，即可在日常协作中**主动捕捉**用户长期偏好、架构拍板、职责/拓扑与事实更正并顺手调用 `world remember`，同时**自动过滤**一次性指令、临时报错堆栈与敏感密钥等噪音，并在缺乏背景或需要历史状态时**主动调用 `world recall`**。

---

## 登录与授权

前往 [World Machine 控制台](https://console.worldmachine.dev) 获取账号后，支持以下两种授权方式：

```sh
# 方式 A：浏览器一键授权（推荐）
world auth login

# 若在远程 SSH / 无桌面环境，打印授权链接并在浏览器确认后粘贴兜底命令
world auth login --no-browser

# 方式 B：直接配置控制台创建的 API Key
world auth login --api-key "<YOUR_API_KEY>"

# 检查配置、服务连通性与密钥有效性（支持 --offline 与 --json）
world auth status
```

凭据默认以 `0600` 权限保存在 `~/.config/world/config.json`（或 `$XDG_CONFIG_HOME/world/config.json`），终端与日志输出中会自动脱敏。

---

## 核心命令

### 1. 写入记忆（`world remember`）

接收单行或多行自然语言陈述，自动附带本机 IANA 时区并生成幂等键：

```sh
# 日常实时记录（保留“昨天”“上个月”等相对时间原话即可）
world remember "我上个月搬到上海，现在在 Acme 当架构师。" --source cli

# 补录历史事件并附带元信息标签
world remember "项目代号正式定为 Atlas。" \
  --occurred-at "2026-01-15" \
  --timezone "Asia/Shanghai" \
  --meta project=atlas \
  --idempotency-key "atlas-kickoff-001"

# 从标准输入管道写入多行文本并以 JSON 输出结果
cat meeting-note.txt | world remember - --source meeting --json
```

| 参数 | 说明 |
| --- | --- |
| `--source <label>` | 来源标签（如 `cli`、`agent`、`notes`、`meeting`） |
| `--timezone <iana>` | 说话人 IANA 时区（默认自动检测本机时区），用于解析相对时间词 |
| `--occurred-at <time>` | 实际发生时间（支持 `YYYY-MM-DD`、`YYYY-MM-DD HH:MM:SS`、RFC3339 或 Unix 时间戳） |
| `--meta k=v` | 附加键值元信息留档（可多次传入） |
| `--idempotency-key <key>` | 显式指定幂等键；省略时每次命令调用自动生成并在网络重试时复用 |
| `--json` | 输出 JSON：`{"deduplicated":false}` |

### 2. 自然语言召回（`world recall`）

按自然语言问题同时召回最相关的结构化实体（`entities`）与原始记忆（`memories`）：

```sh
# 人类可读格式输出
world recall "我现在在哪工作？"

# 按历史时间点回溯 + JSON 输出（供 Agent 或脚本解析）
world recall "当时我在哪工作？" --as-of "2025-10-01" --json

# 限制返回条数以控制上下文长度
world recall "Atlas 项目进展" --max-entities 5 --max-events 10 --json
```

| 参数 | 说明 |
| --- | --- |
| `--as-of <time>` | 查询该历史时刻成立的状态快照（支持 `YYYY-MM-DD`、`YYYY-MM-DD HH:MM:SS`、RFC3339 或 Unix 时间戳） |
| `--max-entities <n>` | 返回实体数量上限（1..100，默认服务端缺省值） |
| `--max-events <n>` | 返回原文记忆数量上限（1..100，默认服务端缺省值） |
| `--timezone <iana>` | 指定时间展示与 `--as-of` 纯日期解析所用的 IANA 时区 |
| `--json` | 输出完整结构化 JSON（推荐在 AI Agent 与自动化脚本中始终开启） |

### 3. 查看单个实体（`world entity get`）

从召回结果获得 `entityId` 后，精确查看该实体的全部属性或历史某时刻的快照：

```sh
world entity get ce7b7b0c-4dbd-5524-9392-13067cf6415c
world entity get ce7b7b0c-4dbd-5524-9392-13067cf6415c --as-of "2026-01-15" --json
```

### 4. 多环境配置管理（`world config`）

支持通过 `profile` 切换多套账号或自定义默认参数：

```sh
# 查看当前生效配置（API Key 自动脱敏）
world config list
world config get api-key

# 设置默认时区、默认来源标签或默认输出格式（text / json）
world config set timezone Asia/Shanghai
world config set source agent
world config set output json

# 切换或新建 profile（如区分个人与工作账号）
world config set profile work
world auth login
```

---

## 环境变量与退出码

### 环境变量

| 环境变量 | 说明 |
| --- | --- |
| `WORLD_API_KEY` / `WM_API_KEY` | 临时覆盖当前使用的 API Key |
| `WORLD_PROFILE` | 临时指定生效的配置档名称 |
| `WORLD_TIMEZONE` | 覆盖默认 IANA 时区（如 `Asia/Shanghai`） |
| `WORLD_SOURCE` | 覆盖 `world remember` 的默认来源标签 |
| `WORLD_ENDPOINT` | 覆盖 API 服务地址（默认 `https://api.worldmachine.dev`） |
| `WORLD_CONSOLE_URL` | 覆盖控制台地址（默认 `https://console.worldmachine.dev`） |

### 退出码

| 退出码 | 含义 |
| --- | --- |
| `0` | 执行成功 |
| `1` | 网络、服务端或一般运行错误 |
| `2` | 命令用法或参数格式错误 |
| `3` | 未登录或 API 密钥无效/已撤销 |
| `4` | 请求的实体不存在，或在指定历史时间点尚未创建 |

---

## 相关链接

- **官方网站**：<https://worldmachine.dev>
- **5 分钟快速开始**：<https://worldmachine.dev/tutorial.html>
- **CLI 与 Skill 完整手册**：<https://worldmachine.dev/cli.html>
- **核心概念与使用指南**：<https://worldmachine.dev/concepts.html>
- **开发者控制台**：<https://console.worldmachine.dev>
