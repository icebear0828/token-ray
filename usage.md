# AI Flight Dashboard 使用手册

本文档面向当前项目根目录公开入口。`docs/` 下的文档可能包含历史设计、路线图和内部说明，使用方式以本文档和 `README.md` 为准。

## 安装

### macOS 桌面版

从 GitHub Releases 下载对应的 macOS 包，解压后首次启动请右键 `AI Flight Dashboard.app`，选择“打开”。

### 服务器部署

在服务器或后台环境中，可以使用部署脚本编译并注册 systemd 服务：

```bash
chmod +x ./scripts/deploy.sh
sudo ./scripts/deploy.sh
```

脚本会提示选择：

- 主控服务端：启动 Web 面板并接收探针上报。
- 探针端：监听本机 AI 工具日志，并通过 `/api/track` 上报到主控。

部署后常用命令：

```bash
systemctl status ai-flight-dashboard
journalctl -fu ai-flight-dashboard
```

## 构建

```bash
go build -o dashboard ./cmd/dashboard
```

前端单独构建：

```bash
cd frontend
npm install
npm run build
```

## 启动模式

### 默认 GUI 模式

```bash
./dashboard
```

当前版本默认启动 Wails 桌面 GUI。只有显式指定 Web、TUI、forwarder 或子命令时，才不会进入默认 GUI 模式。

### Web 模式

```bash
./dashboard --web
./dashboard --web --port 8080 --device-id my-mac
```

默认监听 `0.0.0.0:19100`，本机浏览器访问：

```text
http://localhost:19100
```

### 旧 TUI 模式

```bash
./dashboard --tui
```

TUI 是 legacy 模式，适合终端侧栏或 tmux 分屏使用。

### Forwarder 探针模式

```bash
DASHBOARD_TOKEN=your-token ./dashboard \
  --device-id server-a \
  --forward-to http://master-ip:19100/api/track
```

探针会监听本机 Claude Code 和 Gemini CLI 日志，并将用量上报到主控。主控如果配置了 token，探针需要使用相同的 `DASHBOARD_TOKEN` 或 `--token`。

## 常用参数

| 参数 | 说明 |
|---|---|
| `--web`, `-w` | 启动 Web 面板 |
| `--tui` | 启动 legacy TUI |
| `--port`, `-p` | Web 端口，默认 `19100` |
| `--device-id` | 当前设备 ID，默认主机名 |
| `--data-dir` | 数据库和配置目录，默认 `~/.ai-flight-dashboard`；便携模式请显式传目录 |
| `--token` | API、forwarder、LAN 同步认证 token |
| `--forward-to` | 探针上报目标，例如 `http://host:19100/api/track` |
| `--lan` | 启用 LAN 发现和广播，默认开启 |
| `--sync-mode` | 同步模式：`poll`、`fsnotify`、`once` |
| `--billing-mode` | 计费模式：`auto`、`subscription`、`api` |
| `--plan` | 订阅计划：`pro`、`max5`、`max20` |
| `--budget-daily` | API 模式每日预算，`0` 表示关闭 |

`--token` 也可以通过环境变量提供：

```bash
export DASHBOARD_TOKEN=your-token
```

`--data-dir` 也可以通过环境变量提供，命令行参数优先级更高：

```bash
export AI_FLIGHT_DASHBOARD_DATA_DIR="$HOME/.ai-flight-dashboard"
```

## 捕获来源

当前实现支持以下来源：

- Claude Code：监听 `~/.claude/projects/**/*.jsonl`
- Gemini CLI：监听 `~/.gemini/tmp/**/*.jsonl`
- Codex：从 `~/.codex/logs_2.sqlite` 读取 telemetry，并使用 `~/.codex/state_5.sqlite` 解析项目路径
- DeepSeek Harness：监听 `$DSH_HOME/sessions/**/session[.vN].jsonl[.zstd]`（默认 `~/.dsh`），支持 zstd
- Pi Agent：监听 `$PI_CODING_AGENT_DIR/sessions/**/*.jsonl`（默认 `~/.pi/agent`）

GUI 设置页可以配置额外监听目录。新增或移除目录后保存配置，运行中的 watcher 会动态更新。

## 数据与配置

默认数据目录：

- 默认使用 `~/.ai-flight-dashboard`。
- 显式传 `--data-dir` 时使用指定目录。
- 未传 `--data-dir` 时，可用 `AI_FLIGHT_DASHBOARD_DATA_DIR` 覆盖默认目录。

主要文件：

```text
stats/usage.db                         # SQLite 用量数据库
config.json                            # 应用配置
custom_pricing.json                    # 自定义模型价格
```

Dashboard 会在 data-dir 下创建 `dashboard.lock`，避免多个本地进程同时写入同一个数据库。

## Pricing

启动时价格来源按优先级合并：

1. 尝试从 GitHub 拉取动态 `pricing_table.json`。
2. 拉取失败时使用嵌入在二进制中的 `cmd/dashboard/pricing_table.json`。
3. 加载 data-dir 下的 `custom_pricing.json` 覆盖或补充模型价格。

Web/GUI 设置页会通过 `/api/pricing` 保存自定义价格到 `custom_pricing.json`。

## LAN 同步

LAN 发现默认开启。没有 token 时只做发现和实时广播；要启用认证数据库同步，需要设置 `--token` 或 `DASHBOARD_TOKEN`。

```bash
DASHBOARD_TOKEN=your-token ./dashboard --web --port 19100
```

可用接口：

```text
GET  /api/lan/scan
POST /api/lan/join
GET  /api/sync/pull
```

## 数据维护

### 修复历史数据

```bash
./dashboard repair-history
./dashboard --data-dir ~/.ai-flight-dashboard repair-history
```

`repair-history` 会重新扫描本机可访问的 Claude Code、Gemini CLI 和 Codex 历史日志。它会重置可重放文件的扫描 offset，并将可从磁盘重放的本机 Gemini 旧记录标记为 superseded，不会物理删除记录，也不会影响 LAN 或远端设备记录。

### 导出 CSV

```bash
./dashboard export > usage.csv
./dashboard --device-id my-mac export > usage-my-mac.csv
```

### 导入 CSV

```bash
./dashboard import usage.csv
```

导入会跳过重复记录。

### 去重

```bash
./dashboard dedup
```

用于清理历史重复记录。执行前建议先导出 CSV 备份。

### Antigravity CLI statusline

```bash
./dashboard antigravity-statusline
```

该命令从 stdin 读取 Antigravity CLI `/statusline` JSON，记录当前会话 token 用量，并输出一行状态文本。

## HTTP API 摘要

```text
GET  /api/stats
GET  /api/stats?device={device_id}
GET  /api/stats?source={source_name}
GET  /api/cache-savings
GET  /api/pricing
PUT  /api/pricing
GET  /api/config
PUT  /api/config
POST /api/track
POST /api/device-alias
POST /api/pause
GET  /download/dashboard
GET  /install.sh
```

`source_name` 常见值：

- `Claude Code`
- `Gemini CLI`
- `Codex`
- `Antigravity`
- `DeepSeek Harness`
- `Pi Agent`

需要认证的写接口使用 bearer token：

```bash
curl -H "Authorization: Bearer $DASHBOARD_TOKEN" http://localhost:19100/api/stats
```

## 测试

Go 测试：

```bash
go test ./...
```

前端 E2E：

```bash
cd frontend
npm run test:e2e
```
