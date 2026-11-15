# Token Ray - PRD

## 1. 系统愿景 (System Vision)
打造一个极致轻量、零依赖的“Token Ray”命令行工具（基于 Go 语言构建）。
利用“被动雷达监听”技术，自动识别当前活跃的 AI CLI 工具（Gemini, Claude Code, Aider/Cursor 等），在终端中提供丝滑的实时消耗反馈（HUD 模式），并能在终端内直接切换到基于 SQLite 的全量资产分析视图（Dashboard 模式）。

## 2. 技术栈选型 (Technology Stack)
* **核心语言**: Go (Golang) - 极速编译、极低内存占用、单一二进制文件零依赖分发。
* **终端 UI (TUI)**: `charmbracelet/bubbletea` 驱动交互，搭配 `lipgloss` 进行精致的样式渲染。
* **文件监听**: `fsnotify` 库，实现低延迟无阻塞的日志流监听（Passive Radar）。
* **本地存储**: `mattn/go-sqlite3` 驱动，用于冷数据脱水持久化。
* **测试驱动 (TDD)**: Go 内置的 `testing` 框架，保证“TDD E2E优先”。

## 3. 核心功能与捕获机制

### 3.1 被动雷达模式 (Passive Radar)
系统通过文件系统的 `fsnotify` 机制，无侵入式读取各引擎的原始日志，不干涉工具原有的 stdin/stdout。

| 引擎 | 日志位置特征 (macOS 适配) | 捕获逻辑 | 提取指标 |
| :--- | :--- | :--- | :--- |
| **Claude Code** | `~/.claude/projects/*/*.jsonl` | `fsnotify` 增量读取，解析 `type: "assistant"` | 输入、缓存读取、输出 Tokens |
| **Gemini CLI** | `~/.gemini/tmp/*/chats/session-*.jsonl` | `fsnotify` 监听，解析 `type: "gemini"` | 输入、缓存命中、思考、输出 Tokens |
| **Aider / Cursor**| 工作目录 `.aider.chat.history.md` | 增量扫描正则标签 | 对应交互 Tokens |

### 3.2 计费与数据口径
- **New Input (计费输入)** = `Total Input` - `Cached Input`
- **Cache Hit Rate** = `Cached Input` / `Total Input` * 100%
- **Real-time Cost** = 依据内置的 `pricing_table.json`，根据不同模型（如 `gemini-2.5-pro`, `claude-3-7-sonnet`）精确计算实时美金消耗。

## 4. 架构与目录组织

```text
/Users/c/wiki/token/token-ray/
├── cmd/
│   └── dashboard/
│       └── main.go           # CLI 入口
├── internal/
│   ├── tui/                # Bubble Tea UI 模型 (HUD 与 Dashboard 视图)
│   ├── watcher/            # fsnotify 增量监听逻辑
│   ├── calculator/         # Token 与 Cost 计算引擎
│   └── db/                 # SQLite 持久层接口与 Schema
├── stats/
│   ├── pricing_table.json  # 费率表配置
│   └── usage.db            # SQLite 数据库文件 (运行时生成)
├── go.mod
├── go.sum
└── PRD.md                  # 本文档
```

## 5. 实施路线图 (Roadmap)

### 第一阶段：极客 HUD 层 (即将开始 - TDD 优先)
* **目标**：实现类似 `claude-hud` 的“常驻底部”视觉效果。
* **交付物**：利用 Bubble Tea 实现的终端面板，运行后能立刻捕获新日志并闪烁更新 Token 与金额消耗。

### 第二阶段：结构化持久层 
* **目标**：日志“脱水”入库。
* **交付物**：在 Go 内部实现一个后台 goroutine，将捕获到的有效条目 Upsert 进 `stats/usage.db`。

### 第三阶段：全键盘终端看板
* **目标**：直接在终端里画表，替换原定的 Streamlit 方案。
* **交付物**：通过快捷键（如 `Tab`）将 HUD 模式切换为大屏 Dashboard 模式。展示：
  1. 近 7 天消耗趋势图（ASCII 图表）。
  2. 烧钱项目排行榜。
  3. 缓存命中率曲线。

## 6. 使用指南
* **实时模式**：`go run ./cmd/dashboard`，放置在终端侧边栏或底部 Tmux 分屏。
* **快捷键**：
  * `q` 或 `Ctrl+C` 退出
  * `Tab` 切换 HUD / Dashboard 分析视图

## 7. 规则约束
* **TDD e2e 优先**：任何核心模块必须带有 `_test.go`。
* **代码简洁至上**：极致利用 Go 标准库，保持依赖树干净。
