# AI Flight Dashboard

## Types

```typescript
interface PeriodCost {
  label: string
  cost: number
  input_tokens: number
  cached_tokens: number
  cache_creation_tokens: number
  output_tokens: number
  cache_hit_rate: number
}

interface ModelStats {
  model: string
  events: number
  input_tokens: number
  cached_tokens: number
  cache_creation_tokens: number
  output_tokens: number
  total_cost: number
  cache_hit_rate: number
  input_price_per_m: number
  cached_price_per_m: number
  cache_creation_price_per_m: number
  output_price_per_m: number
}

interface SourceStats {
  name: string
  total_input: number
  total_cached: number
  total_cache_creation: number
  total_output: number
  total_cost: number
  total_events: number
  cache_hit_rate: number
  models: ModelStats[]
}

interface ProjectStat {
  project: string
  events: number
  input_tokens: number
  cached_tokens: number
  cache_creation_tokens: number
  output_tokens: number
  total_cost: number
  cache_hit_rate: number
}

interface DailyCostBucket {
  date: string
  cost: number
}

interface CalendarHeatmapBucket {
  date: string
  tokens: number
  cost: number
  events: number
}

interface ToolShareBucket {
  source: string
  tokens: number
  total_cost: number
  events: number
}

interface StatsCharts {
  daily_costs: DailyCostBucket[]
  calendar_heatmap: CalendarHeatmapBucket[]
  tool_share: ToolShareBucket[]
}

interface StatsResponse {
  periods: PeriodCost[]
  charts: StatsCharts
  sources: SourceStats[]
  projects: ProjectStat[]
  devices: Array<{ id: string; display_name: string }>
  is_paused: boolean
}

interface LANStatusResponse {
  enabled: boolean
}

interface SystemLogsResponse {
  path: string
}
```

## Endpoints

### 1. Get Live Stats
```
GET /api/stats
GET /api/stats?device={device_id}
GET /api/stats?source={source_name}
GET /api/stats?device={device_id}&source={source_name}
GET /api/stats?device={device_id}&source={source_name}&detail={full|summary|details}
```

`source_name` supports the same source names shown in the dashboard, including `Claude Code`, `Gemini CLI`, `Codex`, and `Antigravity`.

`detail` is optional. Omit it, or use `full`, to return the complete legacy payload plus chart aggregates. Use `summary` for a lightweight response containing periods, chart aggregates, source totals, devices, and pause state. Use `details` for model/project details that can be loaded after the summary.

Response:
```json
{
  "periods": [
    {
      "label": "1h",
      "cost": 0.5,
      "input_tokens": 120000,
      "cached_tokens": 30000,
      "cache_creation_tokens": 5000,
      "output_tokens": 8000,
      "cache_hit_rate": 25.0
    }
  ],
  "charts": {
    "daily_costs": [
      { "date": "2026-06-08", "cost": 0.45 }
    ],
    "calendar_heatmap": [
      { "date": "2026-06-08", "tokens": 17000, "cost": 0.45, "events": 10 }
    ],
    "tool_share": [
      { "source": "Claude Code", "tokens": 17000, "total_cost": 0.45, "events": 10 }
    ]
  },
  "sources": [
    {
      "name": "Claude Code",
      "total_input": 15000,
      "total_cached": 5000,
      "total_cache_creation": 1000,
      "total_output": 2000,
      "total_cost": 0.45,
      "total_events": 10,
      "cache_hit_rate": 33.3333333333,
      "models": [
        {
          "model": "claude-3-7-sonnet-20250219",
          "events": 10,
          "input_tokens": 15000,
          "cached_tokens": 5000,
          "cache_creation_tokens": 1000,
          "output_tokens": 2000,
          "total_cost": 0.45,
          "cache_hit_rate": 33.3333333333
        }
      ]
    }
  ],
  "projects": [
    {
      "project": "token",
      "events": 10,
      "input_tokens": 15000,
      "cached_tokens": 5000,
      "cache_creation_tokens": 1000,
      "output_tokens": 2000,
      "total_cost": 0.45,
      "cache_hit_rate": 33.3333333333
    }
  ],
  "devices": [{ "id": "local", "display_name": "local" }],
  "is_paused": false
}
```

`cache_hit_rate` is `cached_tokens / input_tokens * 100`, bounded to `0..100`.

`charts.daily_costs` and `charts.calendar_heatmap` are fixed 30-day UTC calendar-day series in ascending date order. `charts.tool_share` is sorted by total tokens descending and uses `input_tokens + output_tokens` as the token total.

### 2. LAN Runtime Status
```
GET /api/lan/status
```

Response:
```json
{ "enabled": true }
```

### 3. Join LAN Runtime
```
POST /api/lan/join
```

Starts LAN discovery/sync when it is disabled, or sends an immediate LAN ping when it is already running.

Response:
```json
{ "enabled": true }
```

### 4. Leave LAN Runtime
```
POST /api/lan/leave
```

Stops LAN discovery/sync for the running process and persists `enable_lan=false`.

Response:
```json
{ "enabled": false }
```

### 5. Get System Logs Path
```
GET /api/system/logs
```

Response:
```json
{ "path": "/path/to/ai-flight-dashboard/stats" }
```

## UI Data Flow

```
┌───────────────────────────────────────────────┐
│              DashboardScreen                  │
│                                               │
│  [ Header with Live Indicator ]               │
│                                               │
│  [ Period Cost Cards Row (1h, 24h, ALL...) ]  │
│                                               │
│  [ Source Card ]        [ Source Card ]       │
│  - Total tokens         - Total tokens        │
│  - Donut Chart          - Donut Chart         │
│  - Models Table         - Models Table        │
└───────────────────────────────────────────────┘
```

### 页面布局

- DashboardScreen：全屏监控面板。顶部标题包含 ✈️ AI Flight Dashboard 标题和闪烁的 "LIVE" 徽章。下方先渲染一排 PeriodCost 小卡片。然后渲染 SourceStats 卡片网格。每个 Source 卡片包含该来源的总开销数字、Input/Cached/Output Token 数据的四大块统计（类似原来四个小格）、圆环图（Donut Chart）、以及该来源下的 Models 数据表格。整体遵循深色指挥中心风格，极简但信息密度高。
