export type TimeWindowLabel = "1h" | "24h" | "7d" | "30d" | "3mo" | "6mo" | "1y" | "ALL";

export const TIME_WINDOWS: TimeWindowLabel[] = [
  "1h",
  "24h",
  "7d",
  "30d",
  "3mo",
  "6mo",
  "1y",
  "ALL",
];

export interface PeriodStats {
  label: TimeWindowLabel | string;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens: number;
  output_tokens: number;
  cost: number;
  cache_hit_rate: number; // 0-100
}

export interface ModelStats {
  model: string;
  events: number;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens: number;
  output_tokens: number;
  total_cost: number;
  cache_hit_rate: number;
  input_price_per_m: number;
  cached_price_per_m: number;
  cache_creation_price_per_m: number;
  output_price_per_m: number;
}

export interface SourceStats {
  name: string; // "Claude Code" | "Gemini CLI" | "Codex" | "Antigravity" | ...
  total_input: number;
  total_cached: number;
  total_cache_creation: number;
  total_output: number;
  total_cost: number;
  total_events: number;
  cache_hit_rate: number;
  models: ModelStats[];
}

export interface ProjectStat {
  project: string;
  events: number;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens: number;
  output_tokens: number;
  total_cost: number;
  cache_hit_rate: number;
}

export interface DailyCostPoint {
  date: string;
  cost: number;
}

export interface HeatmapPoint {
  date: string;
  tokens: number;
  cost: number;
  events: number;
}

export interface ToolSharePoint {
  source: string;
  tokens: number;
  total_cost: number;
  events: number;
}

export interface DeviceRef {
  id: string;
  display_name?: string;
}

export interface DashboardData {
  periods: PeriodStats[];
  charts: {
    daily_costs: DailyCostPoint[];
    calendar_heatmap: HeatmapPoint[];
    tool_share: ToolSharePoint[];
  };
  sources: SourceStats[];
  devices: DeviceRef[];
  projects?: ProjectStat[];
  is_paused?: boolean;
}

export interface LanPeerSource {
  source: string;
  tokens_24h?: number;
  tokens_total?: number;
  cost_total?: number;
}

export interface LanPeer {
  id: string;
  display_name?: string;
  ip?: string;
  http_port?: number;
  sync_status?: "ok" | "syncing" | "pending" | "discovery_only" | "error" | string;
  sync_error?: string;
  tokens_24h?: number;
  tokens_total?: number;
  cost_total?: number;
  sources?: LanPeerSource[];
}

export interface LANSelfResponse {
  device_id: string;
  http_port?: number;
}

// Backend shape: internal/web/pricing_handler.go
export interface PricingEntry {
  model: string;
  input_price_per_m: number;
  cached_price_per_m: number;
  cache_creation_price_per_m: number;
  output_price_per_m: number;
}

// Backend shape: internal/config/config.go
export interface AppConfig {
  auto_start: boolean;
  extra_watch_dirs: string[];
  enable_lan?: boolean;
  extra_peer_hosts?: string[];
  tailscale_discovery?: boolean;
}

// Backend shape: internal/model/stats.go DeviceSummary
export interface DeviceSummary {
  id: string;
  display_name: string;
  events: number;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens: number;
  output_tokens: number;
  total_cost: number;
  first_seen?: string;
  last_seen?: string;
}

export interface CacheSavingsResponse {
  actual_cost: number;
  hypothetical_cost: number;
  saved: number;
  saved_percent: number;
  cache_hit_rate: number;
}

export type CurrencyCode = "USD" | "CNY" | "EUR" | "JPY";
