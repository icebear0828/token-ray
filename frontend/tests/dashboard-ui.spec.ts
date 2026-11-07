import { expect, test } from '@playwright/test';

import {
  emptyStatsPayload,
  fulfillConfig,
  fulfillEmptyLANScan,
  fulfillJSON,
  fulfillLANStatus,
  periods,
} from './dashboard-helpers';

test('system logs can be inspected from the config drawer', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, emptyStatsPayload);
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/config', (route) => fulfillConfig(route));
  await page.route('**/api/system/logs', async (route) => {
    await fulfillJSON(route, { path: '/tmp/ai-flight-dashboard/stats' });
  });

  await page.goto('/');

  await page.getByRole('button', { name: '系统配置' }).click();
  await expect(page.getByText('/tmp/ai-flight-dashboard/stats')).toBeVisible();

  await page.getByRole('button', { name: '打开日志目录' }).click();
  await expect(
    page.getByText('系统诊断日志路径: /tmp/ai-flight-dashboard/stats')
  ).toBeVisible();
});

test('dashboard shows cache hit rate in KPIs and attribution tables', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, {
      ...emptyStatsPayload,
      periods: periods.map((period) => ({
        ...period,
        input_tokens: 1000,
        cached_tokens: 250,
        cache_hit_rate: 25,
      })),
      sources: [{
        name: 'Codex',
        total_input: 1000,
        total_cached: 250,
        total_cache_creation: 100,
        total_output: 50,
        total_cost: 1.23,
        total_events: 1,
        cache_hit_rate: 25,
        models: [{
          model: 'gpt-5.5',
          events: 1,
          input_tokens: 1000,
          cached_tokens: 250,
          cache_creation_tokens: 100,
          output_tokens: 50,
          total_cost: 1.23,
          input_price_per_m: 2,
          cached_price_per_m: 0.5,
          cache_creation_price_per_m: 5,
          output_price_per_m: 10,
          cache_hit_rate: 25,
        }],
      }],
      projects: [{
        project: 'token',
        events: 1,
        input_tokens: 1000,
        cached_tokens: 250,
        cache_creation_tokens: 100,
        output_tokens: 50,
        total_cost: 1.23,
        cache_hit_rate: 25,
      }],
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('缓存命中率').first()).toBeVisible();
  await expect(page.getByText('25.0%').first()).toBeVisible();
  await expect(page.getByRole('columnheader', { name: '缓存命中' })).toBeVisible();
});

test('dashboard shows cost intelligence charts', async ({ page }) => {
  const start = new Date('2026-05-10T00:00:00Z');
  const dateAt = (offset: number) => {
    const date = new Date(start);
    date.setUTCDate(start.getUTCDate() + offset);
    return date.toISOString().slice(0, 10);
  };
  const dailyCosts = Array.from({ length: 30 }, (_, index) => ({
    date: dateAt(index),
    cost: index === 28 ? 2.5 : index === 29 ? 12.34 : 0,
  }));
  const calendarHeatmap = Array.from({ length: 30 }, (_, index) => ({
    date: dateAt(index),
    tokens: index === 28 ? 550 : index === 29 ? 1050 : 0,
    cost: index === 28 ? 2.5 : index === 29 ? 12.34 : 0,
    events: index >= 28 ? 1 : 0,
  }));

  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, {
      ...emptyStatsPayload,
      charts: {
        daily_costs: dailyCosts,
        calendar_heatmap: calendarHeatmap,
        tool_share: [
          { source: 'Codex', tokens: 1050, total_cost: 12.34, events: 1 },
          { source: 'Claude Code', tokens: 550, total_cost: 2.5, events: 1 },
        ],
      },
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('近 30 天每日成本')).toBeVisible();
  await expect(page.getByText('每日活跃热力矩阵')).toBeVisible();
  await expect(page.getByText('CLI 工具占比与分布')).toBeVisible();
  await expect(page.getByTestId('daily-cost-bar')).toHaveCount(30);
  await expect(page.getByTestId('daily-cost-bar').first()).toHaveAttribute('data-cost', '0');
  await expect(page.getByTestId('heatmap-cell')).toHaveCount(30);
  await expect(page.getByText('$12.34').first()).toBeVisible();
  await expect(page.getByText('Codex', { exact: true }).first()).toBeVisible();

  await page.getByRole('button', { name: '按 Token' }).click();
  await expect(page.getByText('65.6%')).toBeVisible();
});

test('dashboard shows raw total input tokens alongside cache reads', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, {
      ...emptyStatsPayload,
      periods: periods.map((period) => ({
        ...period,
        input_tokens: 322_400_000,
        cached_tokens: 304_900_000,
        output_tokens: 1_100_000,
        cache_hit_rate: 94.6,
      })),
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('322.40M').first()).toBeVisible();
  await expect(page.getByText('304.90M').first()).toBeVisible();
  await expect(page.getByText('94.6%').first()).toBeVisible();
});

test('dashboard shows nonzero small costs instead of rounding them to zero', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, {
      ...emptyStatsPayload,
      periods: periods.map((period) => ({
        ...period,
        cost: 0.003765,
        input_tokens: 2150,
        cached_tokens: 100,
        cache_creation_tokens: 50,
        output_tokens: 75,
        cache_hit_rate: 4.651162790697675,
      })),
      sources: [{
        name: 'Antigravity',
        total_input: 2150,
        total_cached: 100,
        total_cache_creation: 50,
        total_output: 75,
        total_cost: 0.003765,
        total_events: 1,
        cache_hit_rate: 4.651162790697675,
        models: [{
          model: 'gemini-3.5-flash',
          events: 1,
          input_tokens: 2150,
          cached_tokens: 100,
          cache_creation_tokens: 50,
          output_tokens: 75,
          total_cost: 0.003765,
          input_price_per_m: 1.5,
          cached_price_per_m: 0.15,
          cache_creation_price_per_m: 1.5,
          output_price_per_m: 9,
          cache_hit_rate: 4.651162790697675,
        }],
      }],
      projects: [{
        project: 'token',
        events: 1,
        input_tokens: 2150,
        cached_tokens: 100,
        cache_creation_tokens: 50,
        output_tokens: 75,
        total_cost: 0.003765,
        cache_hit_rate: 4.651162790697675,
      }],
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('$0.0038').first()).toBeVisible();
  await expect(page.getByText('gemini-3.5-flash', { exact: true })).toBeVisible();
});

test('dashboard can filter model and project tables', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, {
      ...emptyStatsPayload,
      sources: [{
        name: 'Codex',
        total_input: 1000,
        total_cached: 250,
        total_cache_creation: 100,
        total_output: 50,
        total_cost: 1.23,
        total_events: 2,
        cache_hit_rate: 25,
        models: [
          {
            model: 'gpt-5.5',
            events: 1,
            input_tokens: 1000,
            cached_tokens: 250,
            cache_creation_tokens: 100,
            output_tokens: 50,
            total_cost: 1.23,
            input_price_per_m: 2,
            cached_price_per_m: 0.5,
            cache_creation_price_per_m: 5,
            output_price_per_m: 10,
            cache_hit_rate: 25,
          },
          {
            model: 'gpt-5.4',
            events: 1,
            input_tokens: 500,
            cached_tokens: 100,
            cache_creation_tokens: 50,
            output_tokens: 25,
            total_cost: 0.42,
            input_price_per_m: 1,
            cached_price_per_m: 0.25,
            cache_creation_price_per_m: 2,
            output_price_per_m: 5,
            cache_hit_rate: 20,
          },
        ],
      }],
      projects: [
        {
          project: 'token',
          events: 1,
          input_tokens: 1000,
          cached_tokens: 250,
          cache_creation_tokens: 100,
          output_tokens: 50,
          total_cost: 1.23,
          cache_hit_rate: 25,
        },
        {
          project: 'codex-proxy',
          events: 1,
          input_tokens: 500,
          cached_tokens: 100,
          cache_creation_tokens: 50,
          output_tokens: 25,
          total_cost: 0.42,
          cache_hit_rate: 20,
        },
      ],
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();

  const search = page.getByPlaceholder('过滤模型或项目...');
  await search.fill('gpt-5.5');
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
  await expect(page.getByText('token', { exact: true })).toHaveCount(0);

  await search.fill('codex-proxy');
  await expect(page.getByText('codex-proxy', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toHaveCount(0);

  await page.getByRole('button', { name: 'TOKENS' }).click();
  await expect(page.getByText('codex-proxy', { exact: true })).toBeVisible();
});
