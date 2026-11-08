import {
  AppConfig,
  DailyCostPoint,
  DashboardData,
  DeviceSummary,
  HeatmapPoint,
  InstalledPlugin,
  LANSelfResponse,
  LanPeer,
  ModelStats,
  PeriodStats,
  PluginIndexEntry,
  PluginManifest,
  PricingEntry,
  ProjectStat,
  SourceDescriptor,
  SourceStats,
  TIME_WINDOWS,
  ToolSharePoint,
} from "../types/telemetry";

const num = (value: unknown): number => {
  const n = typeof value === "number" ? value : Number(value);
  return Number.isFinite(n) ? n : 0;
};

const str = (value: unknown, fallback = ""): string => {
  return typeof value === "string" && value.trim() !== "" ? value : fallback;
};

const record = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === "object" ? (value as Record<string, unknown>) : {};

async function requestJson<T>(url: string, init?: RequestInit): Promise<T> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  try {
    const res = await fetch(url, {
      ...init,
      signal: controller.signal,
      headers: {
        "Content-Type": "application/json",
        ...(init?.headers ?? {}),
      },
    });
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      throw new Error(`HTTP ${res.status}${body ? `: ${body}` : ""}`);
    }
    const contentType = res.headers.get("content-type") ?? "";
    if (!contentType.includes("application/json")) {
      return null as T;
    }
    const text = await res.text();
    if (!text || text.trim() === "") {
      return null as T;
    }
    return JSON.parse(text) as T;
  } finally {
    clearTimeout(timer);
  }
}

const normalizePeriod = (raw: unknown): PeriodStats => {
  const p = record(raw);
  return {
    label: str(p.label, "UNKNOWN"),
    input_tokens: num(p.input_tokens),
    cached_tokens: num(p.cached_tokens),
    cache_creation_tokens: num(p.cache_creation_tokens),
    output_tokens: num(p.output_tokens),
    cost: num(p.cost),
    cache_hit_rate: num(p.cache_hit_rate),
  };
};

const normalizeModel = (raw: unknown): ModelStats => {
  const m = record(raw);
  return {
    model: str(m.model, "unknown"),
    events: num(m.events),
    input_tokens: num(m.input_tokens),
    cached_tokens: num(m.cached_tokens),
    cache_creation_tokens: num(m.cache_creation_tokens),
    output_tokens: num(m.output_tokens),
    total_cost: num(m.total_cost),
    cache_hit_rate: num(m.cache_hit_rate),
    input_price_per_m: num(m.input_price_per_m),
    cached_price_per_m: num(m.cached_price_per_m),
    cache_creation_price_per_m: num(m.cache_creation_price_per_m),
    output_price_per_m: num(m.output_price_per_m),
  };
};

const normalizeSource = (raw: unknown): SourceStats => {
  const s = record(raw);
  const models = Array.isArray(s.models) ? s.models : [];
  return {
    name: str(s.name, "Unknown"),
    total_input: num(s.total_input),
    total_cached: num(s.total_cached),
    total_cache_creation: num(s.total_cache_creation),
    total_output: num(s.total_output),
    total_cost: num(s.total_cost),
    total_events: num(s.total_events),
    cache_hit_rate: num(s.cache_hit_rate),
    models: models.map(normalizeModel),
  };
};

const normalizeProject = (raw: unknown): ProjectStat => {
  const p = record(raw);
  return {
    project: str(p.project, "Default"),
    events: num(p.events),
    input_tokens: num(p.input_tokens),
    cached_tokens: num(p.cached_tokens),
    cache_creation_tokens: num(p.cache_creation_tokens),
    output_tokens: num(p.output_tokens),
    total_cost: num(p.total_cost),
    cache_hit_rate: num(p.cache_hit_rate),
  };
};

export const normalizeDashboardData = (raw: unknown): DashboardData => {
  const root = record(raw);
  const charts = record(root.charts);
  const dailyCosts = Array.isArray(charts.daily_costs) ? charts.daily_costs : [];
  const heatmap = Array.isArray(charts.calendar_heatmap) ? charts.calendar_heatmap : [];
  const toolShare = Array.isArray(charts.tool_share) ? charts.tool_share : [];
  const sources = Array.isArray(root.sources) ? root.sources : [];
  const devices = Array.isArray(root.devices) ? root.devices : [];
  const projects = Array.isArray(root.projects) ? root.projects : [];

  return {
    periods: (Array.isArray(root.periods) ? root.periods : []).map(normalizePeriod),
    charts: {
      daily_costs: dailyCosts.map((rawPoint): DailyCostPoint => {
        const p = record(rawPoint);
        return { date: str(p.date), cost: num(p.cost) };
      }),
      calendar_heatmap: heatmap.map((rawPoint): HeatmapPoint => {
        const p = record(rawPoint);
        return {
          date: str(p.date),
          tokens: num(p.tokens),
          cost: num(p.cost),
          events: num(p.events),
        };
      }),
      tool_share: toolShare.map((rawPoint): ToolSharePoint => {
        const p = record(rawPoint);
        return {
          source: str(p.source, "Unknown"),
          tokens: num(p.tokens),
          total_cost: num(p.total_cost),
          events: num(p.events),
        };
      }),
    },
    sources: sources.map(normalizeSource),
    devices: devices.map((rawDevice) => {
      const d = record(rawDevice);
      const id = str(d.id, str(rawDevice, "local"));
      return { id, display_name: str(d.display_name, id) };
    }),
    projects: projects.map(normalizeProject),
    is_paused: Boolean(root.is_paused),
  };
};

export async function fetchStats(params: {
  device: string;
  source: string;
  detail: "summary" | "details";
}): Promise<DashboardData> {
  const qs = new URLSearchParams({
    device: params.device || "all",
    source: params.source || "",
    detail: params.detail,
  });
  const raw = await requestJson<unknown>(`/api/stats?${qs.toString()}`);
  if (raw === null) {
    throw new Error("Dashboard returned an empty or non-JSON response");
  }
  return normalizeDashboardData(raw);
}

export async function togglePause(): Promise<{ is_paused: boolean }> {
  const res = await requestJson<{ is_paused: boolean }>("/api/pause", { method: "POST" });
  return { is_paused: Boolean(res?.is_paused) };
}

export async function fetchLanStatus(): Promise<{ enabled: boolean }> {
  const res = await requestJson<{ enabled: boolean }>("/api/lan/status");
  return { enabled: Boolean(res?.enabled) };
}

export async function fetchLanScan(): Promise<{ peer_infos: LanPeer[] }> {
  const res = await requestJson<{ peer_infos?: LanPeer[]; peers?: string[] }>("/api/lan/scan");
  if (Array.isArray(res?.peer_infos)) {
    return { peer_infos: res.peer_infos };
  }
  if (Array.isArray(res?.peers)) {
    return { peer_infos: res.peers.map((id) => ({ id })) };
  }
  return { peer_infos: [] };
}

export async function fetchLANSelf(): Promise<LANSelfResponse> {
  const res = await requestJson<LANSelfResponse>("/api/lan/self");
  return { device_id: str(res?.device_id), http_port: res?.http_port };
}

export async function joinLan(): Promise<void> {
  await requestJson<unknown>("/api/lan/join", { method: "POST" });
}

export async function leaveLan(): Promise<void> {
  await requestJson<unknown>("/api/lan/leave", { method: "POST" });
}

const normalizePricing = (raw: unknown): PricingEntry => {
  const p = record(raw);
  return {
    model: str(p.model, "unknown"),
    input_price_per_m: num(p.input_price_per_m),
    cached_price_per_m: num(p.cached_price_per_m),
    cache_creation_price_per_m: num(p.cache_creation_price_per_m),
    output_price_per_m: num(p.output_price_per_m),
  };
};

export async function fetchPricing(): Promise<PricingEntry[]> {
  const res = await requestJson<unknown>("/api/pricing");
  return Array.isArray(res) ? res.map(normalizePricing) : [];
}

export async function savePricing(entries: PricingEntry[]): Promise<void> {
  await requestJson<unknown>("/api/pricing", {
    method: "PUT",
    body: JSON.stringify(entries.map(normalizePricing)),
  });
}

const normalizeConfig = (raw: unknown): AppConfig => {
  const c = record(raw);
  return {
    auto_start: Boolean(c.auto_start),
    extra_watch_dirs: Array.isArray(c.extra_watch_dirs) ? c.extra_watch_dirs.map(String) : [],
    enable_lan: c.enable_lan === undefined ? true : Boolean(c.enable_lan),
    extra_peer_hosts: Array.isArray(c.extra_peer_hosts) ? c.extra_peer_hosts.map(String) : [],
    tailscale_discovery: c.tailscale_discovery === undefined ? true : Boolean(c.tailscale_discovery),
    disabled_plugins: Array.isArray(c.disabled_plugins) ? c.disabled_plugins.map(String) : [],
    plugin_index_url: str(c.plugin_index_url),
  };
};

export async function fetchConfig(): Promise<AppConfig> {
  const res = await requestJson<unknown>("/api/config");
  return normalizeConfig(res);
}

export async function saveConfig(cfg: AppConfig): Promise<void> {
  await requestJson<unknown>("/api/config", {
    method: "PUT",
    body: JSON.stringify(cfg),
  });
}

export async function fetchDevices(): Promise<DeviceSummary[]> {
  const res = await requestJson<unknown>("/api/devices");
  if (!Array.isArray(res)) return [];
  return res.map((rawDevice) => {
    const d = record(rawDevice);
    const id = str(d.id, "unknown");
    return {
      id,
      display_name: str(d.display_name, id),
      events: num(d.events),
      input_tokens: num(d.input_tokens),
      cached_tokens: num(d.cached_tokens),
      cache_creation_tokens: num(d.cache_creation_tokens),
      output_tokens: num(d.output_tokens),
      total_cost: num(d.total_cost),
      first_seen: str(d.first_seen),
      last_seen: str(d.last_seen),
    };
  });
}

export async function softDeleteDevice(deviceId: string): Promise<void> {
  const qs = new URLSearchParams({ device_id: deviceId });
  await requestJson<unknown>(`/api/devices?${qs.toString()}`, { method: "DELETE" });
}

export async function setDeviceAlias(deviceId: string, displayName: string): Promise<void> {
  await requestJson<unknown>("/api/device-alias", {
    method: "POST",
    body: JSON.stringify({ device_id: deviceId, display_name: displayName }),
  });
}

export async function deleteDeviceAlias(deviceId: string): Promise<void> {
  const qs = new URLSearchParams({ device_id: deviceId });
  await requestJson<unknown>(`/api/device-alias?${qs.toString()}`, { method: "DELETE" });
}

export async function fetchSystemLogsPath(): Promise<{ path: string }> {
  const res = await requestJson<{ path: string }>("/api/system/logs");
  return { path: str(res?.path) };
}

type WailsWindow = Window & {
  go?: {
    desktop?: {
      App?: {
        OpenSystemLogs?: () => Promise<void> | void;
      };
    };
  };
};

export function isDesktopRuntime(): boolean {
  return typeof window !== "undefined" && Boolean((window as WailsWindow).go);
}

export async function openSystemLogs(): Promise<string> {
  const app = (window as WailsWindow).go?.desktop?.App;
  if (app?.OpenSystemLogs) {
    await Promise.resolve(app.OpenSystemLogs());
    return "";
  }
  const res = await fetchSystemLogsPath();
  return res.path;
}

export function defaultPeriods(): PeriodStats[] {
  return TIME_WINDOWS.map((label) => ({
    label,
    input_tokens: 0,
    cached_tokens: 0,
    cache_creation_tokens: 0,
    output_tokens: 0,
    cost: 0,
    cache_hit_rate: 0,
  }));
}

const normalizeManifest = (raw: unknown): PluginManifest => {
  const m = record(raw);
  return {
    schema_version: num(m.schema_version),
    id: str(m.id),
    name: str(m.name, str(m.id)),
    source: str(m.source, str(m.name)),
    label: str(m.label),
    version: str(m.version),
    min_app_version: str(m.min_app_version),
    description: str(m.description),
    author: str(m.author),
    homepage: str(m.homepage),
  };
};

const normalizeIndexEntry = (raw: unknown): PluginIndexEntry => {
  const e = record(raw);
  return {
    id: str(e.id),
    name: str(e.name, str(e.id)),
    version: str(e.version),
    description: str(e.description),
    min_app_version: str(e.min_app_version),
    download_url: str(e.download_url),
    sha256: str(e.sha256),
  };
};

export async function fetchSources(): Promise<SourceDescriptor[]> {
  const res = await requestJson<unknown>("/api/sources");
  if (!Array.isArray(res)) return [];
  return res.map((raw) => {
    const d = record(raw);
    return {
      id: str(d.id),
      source: str(d.source),
      label: str(d.label, str(d.source)),
      description: str(d.description),
      version: str(d.version),
      builtin: Boolean(d.builtin),
    };
  });
}

export async function fetchPlugins(): Promise<InstalledPlugin[]> {
  const res = await requestJson<unknown>("/api/plugins");
  if (!Array.isArray(res)) return [];
  return res.map((raw) => {
    const p = record(raw);
    return {
      manifest: normalizeManifest(p.manifest),
      path: str(p.path),
      enabled: Boolean(p.enabled),
    };
  });
}

export async function installPluginZip(file: File): Promise<PluginManifest> {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch("/api/plugins/install", { method: "POST", body: form });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`HTTP ${res.status}${body ? `: ${body}` : ""}`);
  }
  return normalizeManifest(await res.json());
}

export async function installPluginURL(url: string, sha256 = ""): Promise<PluginManifest> {
  const res = await requestJson<unknown>("/api/plugins/install", {
    method: "POST",
    body: JSON.stringify({ url, sha256 }),
  });
  return normalizeManifest(res);
}

export async function fetchPluginIndex(): Promise<PluginIndexEntry[]> {
  const res = await requestJson<unknown>("/api/plugins/index");
  const rows = Array.isArray(res)
    ? res
    : Array.isArray(record(res).plugins)
      ? (record(res).plugins as unknown[])
      : [];
  return rows.map(normalizeIndexEntry);
}

export async function removePlugin(id: string): Promise<void> {
  await requestJson<unknown>(`/api/plugins/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function setPluginEnabled(id: string, enabled: boolean): Promise<void> {
  await requestJson<unknown>(
    `/api/plugins/${encodeURIComponent(id)}/${enabled ? "enable" : "disable"}`,
    { method: "POST" }
  );
}
