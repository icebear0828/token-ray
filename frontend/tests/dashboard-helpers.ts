import { type Route } from '@playwright/test';

export const periods = ['1h', '24h', '7d', '30d', '3mo', '6mo', '1y', 'ALL'].map((label) => ({
  label,
  input_tokens: 0,
  cached_tokens: 0,
  cache_creation_tokens: 0,
  output_tokens: 0,
  cost: 0,
  cache_hit_rate: 0,
}));

export const emptyCharts = {
  daily_costs: [],
  calendar_heatmap: [],
  tool_share: [],
};

export const emptyStatsPayload = {
  periods,
  charts: emptyCharts,
  sources: [],
  devices: [{ id: 'local', display_name: 'local' }],
  projects: [],
  is_paused: false,
};

export async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

export async function fulfillEmptyLANScan(route: Route) {
  await fulfillJSON(route, { peers: [], peer_infos: [] });
}

export async function fulfillLANStatus(route: Route, enabled = true) {
  await fulfillJSON(route, { enabled });
}

export async function fulfillLANSelf(route: Route, deviceId = 'local') {
  await fulfillJSON(route, { device_id: deviceId, http_port: 19100 });
}

export async function fulfillConfig(route: Route, overrides: Record<string, unknown> = {}) {
  await fulfillJSON(route, {
    auto_start: false,
    extra_watch_dirs: [],
    enable_lan: true,
    extra_peer_hosts: [],
    tailscale_discovery: true,
    ...overrides,
  });
}

/**
 * The backend's `detail=details` response only carries sources + projects.
 * Keep that shape in tests so the two-stage merge is exercised realistically.
 */
export function detailsPayload(source: string, model: string, cost = 2.5) {
  return {
    periods: [],
    charts: emptyCharts,
    sources: [
      {
        name: source,
        total_input: 1000,
        total_cached: 100,
        total_cache_creation: 50,
        total_output: 25,
        total_cost: cost,
        total_events: 1,
        cache_hit_rate: 10,
        models: [
          {
            model,
            events: 1,
            input_tokens: 1000,
            cached_tokens: 100,
            cache_creation_tokens: 50,
            output_tokens: 25,
            total_cost: cost,
            input_price_per_m: 1,
            cached_price_per_m: 0.1,
            cache_creation_price_per_m: 1,
            output_price_per_m: 2,
            cache_hit_rate: 10,
          },
        ],
      },
    ],
    devices: [],
    projects: [
      {
        project: 'token',
        events: 1,
        input_tokens: 1000,
        cached_tokens: 100,
        cache_creation_tokens: 50,
        output_tokens: 25,
        total_cost: cost,
        cache_hit_rate: 10,
      },
    ],
    is_paused: false,
  };
}
