import { num, text } from "./format";

export type JsonRecord = Record<string, unknown>;

export const asRecord = (value: unknown): JsonRecord => {
  return value !== null && typeof value === 'object' ? value as JsonRecord : {};
};

export interface PeriodStats {
  label: string;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens?: number;
  output_tokens: number;
  cost: number;
  cache_hit_rate: number;
}

export interface SourceModelStats {
  model: string;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens?: number;
  output_tokens: number;
  input_price_per_m?: number;
  cached_price_per_m?: number;
  cache_creation_price_per_m?: number;
  output_price_per_m?: number;
  events: number;
  total_cost: number;
  cache_hit_rate: number;
}

export interface SourceStats {
  name: string;
  total_input: number;
  total_cached: number;
  total_cache_creation?: number;
  total_output: number;
  total_cost: number;
  total_events: number;
  cache_hit_rate: number;
  models?: SourceModelStats[];
}

export interface DeviceStats {
  id: string;
  display_name?: string;
}

export interface ProjectStat {
  project: string;
  events: number;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens?: number;
  output_tokens: number;
  total_cost: number;
  cache_hit_rate: number;
}

export interface DailyCostBucket {
  date: string;
  cost: number;
}

export interface CalendarHeatmapBucket {
  date: string;
  tokens: number;
  cost: number;
  events: number;
}

export interface ToolShareBucket {
  source: string;
  tokens: number;
  total_cost: number;
  events: number;
}

export interface DashboardCharts {
  daily_costs: DailyCostBucket[];
  calendar_heatmap: CalendarHeatmapBucket[];
  tool_share: ToolShareBucket[];
}

export interface DashboardData {
  periods: PeriodStats[];
  charts: DashboardCharts;
  sources: SourceStats[];
  devices: DeviceStats[];
  projects?: ProjectStat[];
  is_paused?: boolean;
}

export const normalizeDashboardData = (raw: unknown): DashboardData => {
  const root = asRecord(raw);
  const periods = Array.isArray(root.periods) ? root.periods : [];
  const sources = Array.isArray(root.sources) ? root.sources : [];
  const devices = Array.isArray(root.devices) ? root.devices : [];
  const projects = Array.isArray(root.projects) ? root.projects : [];
  const charts = asRecord(root.charts);
  const dailyCosts = Array.isArray(charts.daily_costs) ? charts.daily_costs : [];
  const calendarHeatmap = Array.isArray(charts.calendar_heatmap) ? charts.calendar_heatmap : [];
  const toolShare = Array.isArray(charts.tool_share) ? charts.tool_share : [];

  return {
    periods: periods.map((period) => {
      const p = asRecord(period);
      return {
        label: text(p.label, 'UNKNOWN'),
        input_tokens: num(p.input_tokens),
        cached_tokens: num(p.cached_tokens),
        cache_creation_tokens: num(p.cache_creation_tokens),
        output_tokens: num(p.output_tokens),
        cost: num(p.cost),
        cache_hit_rate: num(p.cache_hit_rate),
      };
    }),
    charts: {
      daily_costs: dailyCosts.map((bucket) => {
        const b = asRecord(bucket);
        return {
          date: text(b.date),
          cost: num(b.cost),
        };
      }),
      calendar_heatmap: calendarHeatmap.map((bucket) => {
        const b = asRecord(bucket);
        return {
          date: text(b.date),
          tokens: num(b.tokens),
          cost: num(b.cost),
          events: num(b.events),
        };
      }),
      tool_share: toolShare.map((bucket) => {
        const b = asRecord(bucket);
        return {
          source: text(b.source, 'Unknown'),
          tokens: num(b.tokens),
          total_cost: num(b.total_cost),
          events: num(b.events),
        };
      }),
    },
    sources: sources.map((source) => {
      const src = asRecord(source);
      const models = Array.isArray(src.models) ? src.models : [];
      return {
        name: text(src.name, 'Unknown'),
        total_input: num(src.total_input),
        total_cached: num(src.total_cached),
        total_cache_creation: num(src.total_cache_creation),
        total_output: num(src.total_output),
        total_cost: num(src.total_cost),
        total_events: num(src.total_events),
        cache_hit_rate: num(src.cache_hit_rate),
        models: models.map((model) => {
          const m = asRecord(model);
          return {
            model: text(m.model, 'unknown'),
            input_tokens: num(m.input_tokens),
            cached_tokens: num(m.cached_tokens),
            cache_creation_tokens: num(m.cache_creation_tokens),
            output_tokens: num(m.output_tokens),
            input_price_per_m: num(m.input_price_per_m),
            cached_price_per_m: num(m.cached_price_per_m),
            cache_creation_price_per_m: num(m.cache_creation_price_per_m),
            output_price_per_m: num(m.output_price_per_m),
            events: num(m.events),
            total_cost: num(m.total_cost),
            cache_hit_rate: num(m.cache_hit_rate),
          };
        }),
      };
    }),
    devices: devices.map((device) => {
      const d = asRecord(device);
      const fallbackID = text(device, 'local');
      const id = text(d.id, fallbackID);
      return {
        id,
        display_name: text(d.display_name, id),
      };
    }),
    projects: projects.map((project) => {
      const p = asRecord(project);
      return {
        project: text(p.project, 'Default'),
        events: num(p.events),
        input_tokens: num(p.input_tokens),
        cached_tokens: num(p.cached_tokens),
        cache_creation_tokens: num(p.cache_creation_tokens),
        output_tokens: num(p.output_tokens),
        total_cost: num(p.total_cost),
        cache_hit_rate: num(p.cache_hit_rate),
      };
    }),
    is_paused: Boolean(root.is_paused),
  };
};

export const mergeDashboardDetails = (summary: DashboardData, details: DashboardData): DashboardData => {
  return {
    ...summary,
    sources: details.sources.length > 0 ? details.sources : summary.sources,
    projects: details.projects ?? [],
    is_paused: details.is_paused,
  };
};

export const mergeSummaryWithPreviousDetails = (summary: DashboardData, previous: DashboardData | null): DashboardData => {
  if (!previous) {
    return summary;
  }

  const previousSources = new Map(previous.sources.map((source) => [source.name, source]));
  const sources = summary.sources.map((source) => {
    const previousModels = previousSources.get(source.name)?.models ?? [];
    if (previousModels.length === 0) {
      return source;
    }
    return {
      ...source,
      models: previousModels,
    };
  });

  return {
    ...summary,
    sources,
    projects: summary.projects && summary.projects.length > 0 ? summary.projects : previous.projects ?? [],
  };
};
