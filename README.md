# AI Flight Dashboard

> 本地优先的 AI Token、成本和设备用量仪表盘，支持 Claude Code、Gemini CLI、Codex、Antigravity CLI、DeepSeek Harness 和 Pi Agent。

[English](README.en.md)

AI Flight Dashboard 是一个 Go + React + Wails 应用。它通过被动扫描本机 AI CLI 工具写入的日志和本地数据库，统计 Token、缓存命中、模型成本、项目归因和多设备用量。默认启动桌面 GUI，也可以作为 Web 面板、legacy TUI 或远程 forwarder 运行。

它不需要改造 Claude Code、Gemini CLI、Codex、Antigravity CLI、DeepSeek Harness 或 Pi Agent，也不需要代理真实 API 流量。数据默认保存在本机 `~/.ai-flight-dashboard` 下的 SQLite 数据库中；局域网模式可在多台设备之间发现、同步和去重。

## 当前能力

- 支持 Claude Code、Gemini CLI、Codex、Antigravity CLI、DeepSeek Harness 和 Pi Agent 来源。
- 默认 Wails 桌面 GUI；可切换 Web、legacy TUI、forwarder 探针模式。
- 统计 1h、24h、7d、30d、3mo、6mo、1y、ALL 时间窗口。
- 按 TOTAL、CLAUDE、GEMINI、CODEX、ANTIGRAVITY、DSH、PI 来源切换统计视图。
- 展示项目、模型、设备、Token、缓存读取、缓存写入、输出和总成本。
- 展示缓存命中率，计算口径为 `cached_tokens / input_tokens * 100`。
- 支持项目表和模型表折叠，适合长期运行后的大数据量视图。
- LAN 雷达展示局域网设备，支持加入、退出、同步和手动重新加入。
- 设置页支持模型价格、系统配置、额外监听目录和设备管理。
- 设备支持别名、删除别名和软删除历史设备记录。
- 提供 REST API，可作为本地看板或主控服务端使用。

## 快速开始

### 下载桌面版

从 [GitHub Releases](https://github.com/icebear0828/token-ray/releases/latest) 下载对应平台的压缩包。macOS 首次启动时，如果系统拦截未签名应用，请右键 `AI Flight Dashboard.app` 选择“打开”。

### 本地构建

```bash
go build -o dashboard ./cmd/dashboard
```

常用运行方式：

```bash
# 默认启动桌面 GUI
./dashboard

# Web 模式，浏览器打开 http://localhost:19100
./dashboard --web

# legacy TUI 模式
./dashboard --tui

# 自定义端口和设备 ID
./dashboard --web --port 8080 --device-id my-mac

# 远程探针模式，将本机用量上报到主控
DASHBOARD_TOKEN=your-token ./dashboard \
  --device-id server-a \
  --forward-to http://master-ip:19100/api/track

# 使用当前解析和计费规则重扫本机历史数据
./dashboard repair-history
```

更多部署方式见 [usage.md](usage.md)。

## 数据来源

| 来源 | 默认位置 | 说明 |
|---|---|---|
| Claude Code | `~/.claude/projects/**/*.jsonl` | 解析会话 JSONL、模型、Token 和工作目录归因。 |
| Gemini CLI | `~/.gemini/tmp/**/*.jsonl` | 支持流式日志、`.project_root` 项目归因和增量 offset。 |
| Codex | `~/.codex/sessions/**/*.jsonl`、`~/.codex/logs_2.sqlite`、`~/.codex/state_5.sqlite` | 优先读取 session JSONL 的累计 token usage，缺失时回退 telemetry SQLite，并用 state 数据解析项目路径。 |
| Antigravity CLI | `/statusline` JSON stdin | 读取当前 statusline payload，记录实时 token、缓存 token、模型和项目归因。 |
| DeepSeek Harness (dsh) | `$DSH_HOME/sessions/**/session[.vN].jsonl[.zstd]`（默认 `~/.dsh`） | 解析 dsh 会话记录，支持 zstd、fork/seed 去重和项目归因。 |
| Pi Agent | `$PI_CODING_AGENT_DIR/sessions/**/*.jsonl`（默认 `~/.pi/agent`） | 解析 Pi 会话 JSONL，汇总 assistant、tool、compaction 等计费用量。 |

默认同步模式为 `poll`：首次启动会扫描历史文件，之后快速轮询已知文件并周期性发现新文件。也可使用 `--sync-mode fsnotify` 或 `--sync-mode once`。

## 数据目录与配置

默认数据目录：

```text
~/.ai-flight-dashboard
```

可通过命令行或环境变量覆盖：

```bash
./dashboard --data-dir /path/to/data
AI_FLIGHT_DASHBOARD_DATA_DIR=/path/to/data ./dashboard
```

主要文件：

```text
stats/usage.db          # SQLite 用量数据库
config.json             # 应用配置
custom_pricing.json     # 用户自定义模型价格
dashboard.lock          # 单实例写入锁
```

同一个 data-dir 同一时间只能由一个 Dashboard 进程写入，避免 SQLite 数据竞争。

## 计费与价格

启动时价格按优先级合并：

1. 尝试从 GitHub 获取动态 `pricing_table.json`。
2. 获取失败时使用二进制内嵌的 `cmd/dashboard/pricing_table.json`。
3. 读取 data-dir 下的 `custom_pricing.json` 覆盖或补充模型价格。

Web/GUI 设置页会通过 `/api/pricing` 保存自定义价格。命令行还支持订阅和 API 预算视角：

```bash
./dashboard --billing-mode subscription --plan pro
./dashboard --billing-mode api --budget-daily 20
```

## LAN 与设备管理

LAN 默认开启。未设置 token 时，Dashboard 会做局域网发现和实时广播；设置 `--token` 或 `DASHBOARD_TOKEN` 后，可启用认证同步。

```bash
DASHBOARD_TOKEN=your-token ./dashboard --web --port 19100
```

GUI 设置页可以执行：

- 加入或退出 LAN；
- 查看本机和局域网设备；
- 给设备设置别名；
- 删除设备别名；
- 软删除旧设备记录，历史 usage 会标记为 superseded，不会物理删除。

## 命令与参数

| 命令或参数 | 说明 |
|---|---|
| `./dashboard` | 默认启动 Wails 桌面 GUI。 |
| `--web`, `-w` | 启动 Web 面板。 |
| `--tui` | 启动 legacy TUI。 |
| `--port`, `-p` | Web 端口，默认 `19100`。 |
| `--device-id` | 当前设备 ID，默认主机名。 |
| `--data-dir` | 数据库和配置目录。 |
| `--token` | API、forwarder、LAN 同步认证 token；也可用 `DASHBOARD_TOKEN`。 |
| `--forward-to` | 以探针模式上报到主控 `/api/track`。 |
| `--lan` | 是否启用 LAN 发现和同步，默认开启。 |
| `--sync-mode` | `poll`、`fsnotify` 或 `once`。 |
| `--billing-mode` | `auto`、`subscription` 或 `api`。 |
| `--plan` | 订阅计划：`pro`、`max5`、`max20`。 |
| `--budget-daily` | API 模式每日预算，`0` 表示关闭。 |
| `antigravity-statusline` | 从 Antigravity CLI statusline JSON 读取本次用量，写入本地数据库并输出单行状态。 |
| `repair-history` | 重扫本机 Claude、Gemini、Codex 历史数据并修复统计。 |
| `export` | 导出 CSV 到 stdout。 |
| `import <file.csv>` | 导入 CSV，重复记录会跳过。 |
| `dedup` | 清理历史重复记录，执行前建议先导出备份。 |

## HTTP API

常用接口：

```text
GET    /api/stats
GET    /api/stats?device={device_id}
GET    /api/stats?source={source_name}
GET    /api/stats?detail={full|summary|details}
GET    /api/cache-savings
GET    /api/pricing
PUT    /api/pricing
POST   /api/pricing
GET    /api/config
PUT    /api/config
POST   /api/track
GET    /api/devices
DELETE /api/devices?device_id={device_id}
POST   /api/device-alias
DELETE /api/device-alias?device_id={device_id}
GET    /api/lan/status
GET    /api/lan/self
POST   /api/lan/join
POST   /api/lan/leave
GET    /api/lan/scan
GET    /api/sync/pull
GET    /api/system/logs
POST   /api/pause
GET    /download/dashboard
GET    /install.sh
```

写接口在配置 token 后需要 bearer token：

```bash
curl -H "Authorization: Bearer $DASHBOARD_TOKEN" http://localhost:19100/api/stats
```

完整字段说明见 [docs/dashboard-api.md](docs/dashboard-api.md)。

## 架构

```text
cmd/dashboard/              CLI 入口、运行模式装配、LAN runtime、pricing、repair-history
internal/model/             共享数据结构、计费模式、统计类型
internal/watcher/           实时文件监听、JSONL 解析、项目归因
internal/scanner/           Claude/Gemini 历史扫描、offset、截断容错
internal/codexusage/        Codex sessions、telemetry、threads、SQLite 解析
internal/calculator/        Token 到 USD 成本计算
internal/db/                SQLite 连接、schema、写入、查询、同步、设备、offset
internal/dashboard/         Dashboard stats 聚合和缓存
internal/web/               REST handlers、LAN 控制、设备管理、同步、静态资源
internal/lan/               UDP 广播、监听、HTTP discovery、peer 管理、pull sync
internal/forwarder/         远程探针上报
internal/desktop/           Wails 桌面绑定、系统日志、开机启动
internal/tui/               legacy Bubble Tea HUD
frontend/src/               React Dashboard、设置页、i18n、Wails bridge
scripts/                    部署、桌面构建、Fat Server 构建
```

持久层和 Web/LAN handler 已按职责拆分，业务源文件保持在约 500 行以内，便于后续迭代和 review。

## 开发与质量门

本地行为变更合并前应运行：

```bash
cd frontend && npm run build
cd frontend && npm run test:e2e
go test -race -count=1 -timeout=5m ./...
go vet ./...
go build ./...
```

文档-only 变更只需检查 Markdown 和链接，不需要为未触碰的运行时代码重复跑完整测试。CI 和发布门禁见 [docs/testing_and_ci.md](docs/testing_and_ci.md)。

## 发布

发布通过 GitHub Actions 完成，标准顺序是：

1. 合并 PR 到 `main`。
2. 等待 `main` 上的 `Test` workflow 通过。
3. 运行 `Tag Release` workflow 创建 `vX.Y.Z` tag。
4. 等待 `Release` workflow 构建并上传 Linux、macOS Apple Silicon、macOS Intel、Windows 资产。

详细 runbook 见 [docs/RELEASE.md](docs/RELEASE.md)。

## License

MIT
