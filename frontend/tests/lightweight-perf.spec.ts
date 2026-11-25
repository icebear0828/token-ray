import { expect, test } from '@playwright/test';

import {
  emptyCharts,
  emptyStatsPayload,
  fulfillConfig,
  fulfillEmptyLANScan,
  fulfillJSON,
  fulfillLANStatus,
  periods,
} from './dashboard-helpers';

test('Escape closes an open drawer and restores body scroll', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await page.route('**/api/stats?*', (route) => fulfillJSON(route, emptyStatsPayload));
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/config', (route) => fulfillConfig(route));
  await page.route('**/api/system/version', (route) =>
    fulfillJSON(route, { version: 'v1.13.4', goos: 'darwin', goarch: 'arm64' })
  );
  await page.route('**/api/system/logs', (route) =>
    fulfillJSON(route, { path: '/tmp/ai-flight-dashboard/stats' })
  );

  await page.goto('/');

  await page.getByRole('button', { name: '系统配置' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveAttribute('aria-modal', 'true');
  await expect
    .poll(() => page.evaluate(() => document.body.style.overflow))
    .toBe('hidden');

  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect
    .poll(() => page.evaluate(() => document.body.style.overflow))
    .toBe('');

  expect(pageErrors).toEqual([]);
});

test('language switch EN <-> 中文 updates translated text and interpolated {{period}} strings', async ({
  page,
}) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await page.route('**/api/stats?*', (route) =>
    fulfillJSON(route, {
      ...emptyStatsPayload,
      charts: {
        ...emptyCharts,
        granularity: 'daily',
      },
    })
  );
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('成本与用量趋势')).toBeVisible();
  await expect(page.getByText('30d 时间窗口 · 每日聚合')).toBeVisible();

  await page.getByRole('tab', { name: '7d' }).click();
  await expect(page.getByText('7d 时间窗口 · 每日聚合')).toBeVisible();

  await page.getByRole('button', { name: 'EN', exact: true }).click();
  await expect(page.getByText('COST & USAGE TREND')).toBeVisible();
  await expect(page.getByText('7d WINDOW · DAILY BUCKETS')).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => window.localStorage.getItem('ai-flight-dashboard_lang')))
    .toBe('en');

  await page.getByRole('button', { name: '中文', exact: true }).click();
  await expect(page.getByText('成本与用量趋势')).toBeVisible();
  await expect(page.getByText('7d 时间窗口 · 每日聚合')).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => window.localStorage.getItem('ai-flight-dashboard_lang')))
    .toBe('zh');

  expect(pageErrors).toEqual([]);
});

test('GET /api/stats returning 304 Not Modified on matching If-None-Match preserves rendered dashboard state', async ({
  page,
}) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  const seenIfNoneMatch: string[] = [];
  let notModifiedCount = 0;

  const summaryBody = {
    ...emptyStatsPayload,
    periods: periods.map((period) => ({
      ...period,
      cost: 4.56,
      input_tokens: 2000,
      cached_tokens: 500,
      output_tokens: 150,
      cache_hit_rate: 25,
    })),
    sources: [
      {
        name: 'Codex',
        total_input: 2000,
        total_cached: 500,
        total_cache_creation: 0,
        total_output: 150,
        timed_output_tokens: 0,
        duration_ms: 0,
        timed_events: 0,
        total_cost: 4.56,
        total_events: 2,
        cache_hit_rate: 25,
        models: [],
      },
    ],
  };

  const detailsBody = {
    periods: [],
    charts: emptyCharts,
    sources: [
      {
        name: 'Codex',
        total_input: 2000,
        total_cached: 500,
        total_cache_creation: 0,
        total_output: 150,
        timed_output_tokens: 0,
        duration_ms: 0,
        timed_events: 0,
        total_cost: 4.56,
        total_events: 2,
        cache_hit_rate: 25,
        models: [
          {
            model: 'gpt-5.5',
            events: 2,
            input_tokens: 2000,
            cached_tokens: 500,
            cache_creation_tokens: 0,
            output_tokens: 150,
            timed_output_tokens: 0,
            duration_ms: 0,
            timed_events: 0,
            total_cost: 4.56,
            cache_hit_rate: 25,
            input_price_per_m: 2,
            cached_price_per_m: 0.5,
            cache_creation_price_per_m: 0,
            output_price_per_m: 10,
          },
        ],
      },
    ],
    devices: [],
    projects: [
      {
        project: 'token',
        events: 2,
        input_tokens: 2000,
        cached_tokens: 500,
        cache_creation_tokens: 0,
        output_tokens: 150,
        timed_output_tokens: 0,
        duration_ms: 0,
        timed_events: 0,
        total_cost: 4.56,
        cache_hit_rate: 25,
      },
    ],
    is_paused: false,
  };

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const detail = url.searchParams.get('detail') ?? 'summary';
    const expectedEtag = detail === 'details' ? '"stats-details-v1"' : '"stats-summary-v1"';
    const ifNoneMatch = route.request().headers()['if-none-match'] ?? '';

    if (ifNoneMatch) {
      seenIfNoneMatch.push(ifNoneMatch);
    }

    if (ifNoneMatch === expectedEtag) {
      notModifiedCount += 1;
      await route.fulfill({
        status: 304,
        headers: {
          ETag: expectedEtag,
          'Cache-Control': 'no-cache',
        },
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      headers: {
        ETag: expectedEtag,
        'Cache-Control': 'no-cache',
      },
      body: JSON.stringify(detail === 'details' ? detailsBody : summaryBody),
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('$4.56').first()).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
  await expect(page.getByText('token', { exact: true })).toBeVisible();

  // Wait for the 5-second background poll to revalidate both summary and details via 304.
  await expect.poll(() => notModifiedCount, { timeout: 10000 }).toBeGreaterThanOrEqual(2);
  expect(seenIfNoneMatch).toContain('"stats-summary-v1"');
  expect(seenIfNoneMatch).toContain('"stats-details-v1"');

  await expect(page.getByText('$4.56').first()).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('刷新失败')).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});
