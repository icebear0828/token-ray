# AI Flight Dashboard

> Local-first AI token, cost, and device usage dashboard for Claude Code, Gemini CLI, Codex, Antigravity CLI, DeepSeek Harness, and Pi Agent.

[中文](README.md)

AI Flight Dashboard is a Go + React + Wails app. It passively reads local AI CLI logs and databases to track tokens, cache hits, model costs, project attribution, and multi-device usage. It starts as a native desktop GUI by default, and can also run as a Web dashboard, legacy TUI, or remote forwarder.

It does not require patching Claude Code, Gemini CLI, Codex, Antigravity CLI, DeepSeek Harness, or Pi Agent, and it does not proxy real API traffic. Data is stored locally in SQLite under `~/.ai-flight-dashboard` by default. LAN mode can discover, sync, and deduplicate usage across nearby machines.

## Capabilities

- Supports Claude Code, Gemini CLI, Codex, Antigravity CLI, DeepSeek Harness, and Pi Agent.
- Defaults to a Wails desktop GUI; also supports Web, legacy TUI, and forwarder modes.
- Tracks 1h, 24h, 7d, 30d, 3mo, 6mo, 1y, and ALL windows.
- Switches stats by TOTAL, CLAUDE, GEMINI, CODEX, ANTIGRAVITY, DSH, and PI sources.
- Shows project, model, device, token, cache read, cache creation, output, and cost metrics.
- Shows cache hit rate as `cached_tokens / input_tokens * 100`.
- Collapsible project and model tables for long-running datasets.
- LAN radar for local peers, with join, leave, sync, and rejoin controls.
- Settings UI for pricing, system config, extra watch directories, and device management.
- Device aliases, alias deletion, and soft deletion of old device records.
- REST API for local dashboards or a central receiver service.

## Quick Start

### Download The Desktop App

Download the package for your platform from [GitHub Releases](https://github.com/icebear0828/token-ray/releases/latest). On macOS, if Gatekeeper blocks the first launch, right-click `AI Flight Dashboard.app` and choose "Open".

### Build Locally

```bash
go build -o dashboard ./cmd/dashboard
```

Common run modes:

```bash
# Default native desktop GUI
./dashboard

# Web mode, then open http://localhost:19100
./dashboard --web

# Legacy TUI mode
./dashboard --tui

# Custom port and device ID
./dashboard --web --port 8080 --device-id my-mac

# Remote probe mode, forwarding local usage to a receiver
DASHBOARD_TOKEN=your-token ./dashboard \
  --device-id server-a \
  --forward-to http://master-ip:19100/api/track

# Replay local history with current parser and pricing rules
./dashboard repair-history
```

See [usage.md](usage.md) for more deployment details.

## Data Sources

| Source | Default location | Notes |
|---|---|---|
| Claude Code | `~/.claude/projects/**/*.jsonl` | Parses session JSONL, model, tokens, and workspace attribution. |
| Gemini CLI | `~/.gemini/tmp/**/*.jsonl` | Supports streaming logs, `.project_root` attribution, and incremental offsets. |
| Codex | `~/.codex/sessions/**/*.jsonl`, `~/.codex/logs_2.sqlite`, `~/.codex/state_5.sqlite` | Prefers accumulated token usage from session JSONL, falls back to telemetry SQLite, and uses state data for project paths. |
| Antigravity CLI | `/statusline` JSON stdin | Reads the current statusline payload and records live tokens, cache tokens, model, and project attribution. |
| DeepSeek Harness (dsh) | `$DSH_HOME/sessions/**/session[.vN].jsonl[.zstd]` (default `~/.dsh`) | Parses dsh session records with zstd, fork/seed dedup, and project attribution. |
| Pi Agent | `$PI_CODING_AGENT_DIR/sessions/**/*.jsonl` (default `~/.pi/agent`) | Parses Pi session JSONL and aggregates billed assistant, tool, and compaction usage. |

The default sync mode is `poll`: the app scans history on startup, then quickly polls known files and periodically discovers new files. `--sync-mode fsnotify` and `--sync-mode once` are also available.

## Data Directory And Config

Default data directory:

```text
~/.ai-flight-dashboard
```

Override it with a flag or environment variable:

```bash
./dashboard --data-dir /path/to/data
AI_FLIGHT_DASHBOARD_DATA_DIR=/path/to/data ./dashboard
```

Main files:

```text
stats/usage.db          # SQLite usage database
config.json             # App config
custom_pricing.json     # User-defined model pricing
dashboard.lock          # Single-writer process lock
```

Only one Dashboard process may write to the same data-dir at a time.

## Pricing

Pricing is merged in this order at startup:

1. Try to fetch the dynamic `pricing_table.json` from GitHub.
2. Fall back to the embedded `cmd/dashboard/pricing_table.json`.
3. Apply user overrides from `custom_pricing.json` in the data-dir.

The Web/GUI settings screen saves custom prices through `/api/pricing`. The CLI also supports subscription and API budget views:

```bash
./dashboard --billing-mode subscription --plan pro
./dashboard --billing-mode api --budget-daily 20
```

## LAN And Device Management

LAN is enabled by default. Without a token, Dashboard performs discovery and live broadcasts. With `--token` or `DASHBOARD_TOKEN`, authenticated sync is enabled.

```bash
DASHBOARD_TOKEN=your-token ./dashboard --web --port 19100
```

The GUI settings screen can:

- join or leave LAN;
- show local and LAN devices;
- assign device aliases;
- delete device aliases;
- soft-delete old devices by marking their usage rows as superseded instead of physically deleting them.

## Commands And Flags

| Command or flag | Description |
|---|---|
| `./dashboard` | Start the Wails desktop GUI. |
| `--web`, `-w` | Start the Web dashboard. |
| `--tui` | Start the legacy TUI. |
| `--port`, `-p` | Web port, default `19100`. |
| `--device-id` | Current device ID, default hostname. |
| `--data-dir` | Database and config directory. |
| `--token` | API, forwarder, and LAN sync auth token; can also use `DASHBOARD_TOKEN`. |
| `--forward-to` | Run as a probe and send usage to receiver `/api/track`. |
| `--lan` | Enable LAN discovery and sync, default enabled. |
| `--sync-mode` | `poll`, `fsnotify`, or `once`. |
| `--billing-mode` | `auto`, `subscription`, or `api`. |
| `--plan` | Subscription plan: `pro`, `max5`, or `max20`. |
| `--budget-daily` | Daily API-mode budget, `0` disables it. |
| `antigravity-statusline` | Read Antigravity CLI statusline JSON, store the current usage, and print a one-line status. |
| `repair-history` | Rescan local Claude, Gemini, and Codex history and repair stats. |
| `export` | Export CSV to stdout. |
| `import <file.csv>` | Import CSV and skip duplicates. |
| `dedup` | Remove historical duplicates; export a backup first. |

## HTTP API

Common endpoints:

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

Write endpoints require a bearer token when one is configured:

```bash
curl -H "Authorization: Bearer $DASHBOARD_TOKEN" http://localhost:19100/api/stats
```

See [docs/dashboard-api.md](docs/dashboard-api.md) for response fields.

## Architecture

```text
cmd/dashboard/              CLI entry, runtime wiring, LAN runtime, pricing, repair-history
internal/model/             Shared data structures, billing mode, stats types
internal/watcher/           Live file watching, JSONL parsing, project attribution
internal/scanner/           Claude/Gemini history scanning, offsets, truncation handling
internal/codexusage/        Codex sessions, telemetry, threads, SQLite parsing
internal/calculator/        Token to USD cost calculation
internal/db/                SQLite connection, schema, writes, queries, sync, devices, offsets
internal/dashboard/         Dashboard stats aggregation and cache
internal/web/               REST handlers, LAN control, device management, sync, static assets
internal/lan/               UDP broadcast, listener, HTTP discovery, peer management, pull sync
internal/forwarder/         Remote probe forwarding
internal/desktop/           Wails desktop bindings, system logs, autostart
internal/tui/               Legacy Bubble Tea HUD
frontend/src/               React dashboard, settings UI, i18n, Wails bridge
scripts/                    Deployment, desktop build, Fat Server build
```

Persistence, Web handlers, and LAN code are split by responsibility, keeping business source files near the 500-line target for reviewability.

## Development And Quality Gates

Before merging behavior changes locally:

```bash
cd frontend && npm run build
cd frontend && npm run test:e2e
go test -race -count=1 -timeout=5m ./...
go vet ./...
go build ./...
```

Docs-only changes only need Markdown and link inspection unless they also touch runtime behavior. CI and release gates are documented in [docs/testing_and_ci.md](docs/testing_and_ci.md).

## Release

Releases are created by GitHub Actions:

1. Merge the PR into `main`.
2. Wait for the `Test` workflow on `main`.
3. Run the `Tag Release` workflow to create a `vX.Y.Z` tag.
4. Wait for the `Release` workflow to build and upload Linux, macOS Apple Silicon, macOS Intel, and Windows assets.

See [docs/RELEASE.md](docs/RELEASE.md) for the full runbook.

## License

MIT
