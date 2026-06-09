import { expect, test } from '@playwright/test';

import { fulfillEmptyLANScan, fulfillJSON, fulfillLANStatus, periods } from './dashboard-helpers';

test('system logs click shows a visible fallback in web mode', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, {
      periods,
      sources: [],
      devices: [{ id: 'local', display_name: 'local' }],
      projects: [],
      is_paused: false,
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/system/logs', async (route) => {
    await fulfillJSON(route, { path: '/tmp/ai-flight-dashboard/stats' });
  });

  await page.goto('/');

  await page.getByText('[ 系统日志 ]').click();
  await expect(page.getByText('系统日志路径: /tmp/ai-flight-dashboard/stats')).toBeVisible();
});

test('dashboard shows cache hit rate in stats tables', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, {
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
      devices: [{ id: 'local', display_name: 'local' }],
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
      is_paused: false,
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);

  await page.goto('/');

  await expect(page.getByText('缓存命中率').first()).toBeVisible();
  await expect(page.getByText('25.0%').first()).toBeVisible();
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
      periods,
      charts: {
        daily_costs: dailyCosts,
        calendar_heatmap: calendarHeatmap,
        tool_share: [
          { source: 'Codex', tokens: 1050, total_cost: 12.34, events: 1 },
          { source: 'Claude Code', tokens: 550, total_cost: 2.5, events: 1 },
        ],
      },
      sources: [],
      devices: [{ id: 'local', display_name: 'local' }],
      projects: [],
      is_paused: false,
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);

  await page.goto('/');

  await expect(page.getByRole('heading', { name: '成本洞察' })).toBeVisible();
  await expect(page.getByText('30天每日成本')).toBeVisible();
  await expect(page.getByText('日历热力图')).toBeVisible();
  await expect(page.getByText('工具 Token 占比')).toBeVisible();
  await expect(page.getByTestId('daily-cost-bar')).toHaveCount(30);
  await expect(page.getByTestId('heatmap-cell')).toHaveCount(30);
  await expect(page.getByText('$12.34').first()).toBeVisible();
  await expect(page.getByText('Codex', { exact: true })).toBeVisible();
  await expect(page.getByText('65.6%')).toBeVisible();
});

test('dashboard period cards show raw input tokens without subtracting cache', async ({ page }) => {
	await page.route('**/api/stats?*', async (route) => {
		await fulfillJSON(route, {
      periods: periods.map((period) => ({
        ...period,
        input_tokens: 322_400_000,
        cached_tokens: 304_900_000,
        output_tokens: 1_100_000,
        cache_hit_rate: 94.6,
      })),
      sources: [],
      devices: [{ id: 'local', display_name: 'local' }],
      projects: [],
      is_paused: false,
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);

  await page.goto('/');

  await expect(page.getByText('输入: 322.40M').first()).toBeVisible();
  await expect(page.getByText('缓存读取: 304.90M').first()).toBeVisible();
	await expect(page.getByText('输入: 17.50M')).toHaveCount(0);
});

test('dashboard shows nonzero small costs instead of rounding them to zero', async ({ page }) => {
	await page.route('**/api/stats?*', async (route) => {
		await fulfillJSON(route, {
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
			devices: [{ id: 'local', display_name: 'local' }],
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
			is_paused: false,
		});
	});
	await page.route('**/api/lan/scan', fulfillEmptyLANScan);

	await page.goto('/');

	await expect(page.getByText('$0.0038').first()).toBeVisible();
	await expect(page.getByText('gemini-3.5-flash', { exact: true })).toBeVisible();
});

test('dashboard can collapse project and model tables', async ({ page }) => {
	await page.route('**/api/stats?*', async (route) => {
		await fulfillJSON(route, {
      periods,
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
      devices: [{ id: 'local', display_name: 'local' }],
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
      is_paused: false,
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);

  await page.goto('/');

  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();

  await page.getByRole('button', { name: '收起项目统计' }).click();
  await expect(page.getByText('token', { exact: true })).toBeHidden();
  await expect(page.getByRole('button', { name: '展开项目统计' })).toBeVisible();

  await page.getByRole('button', { name: '收起模型列表' }).click();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeHidden();
  await expect(page.getByRole('button', { name: '展开模型列表' })).toBeVisible();

  await page.getByRole('button', { name: '展开项目统计' }).click();
  await page.getByRole('button', { name: '展开模型列表' }).click();
  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
});
