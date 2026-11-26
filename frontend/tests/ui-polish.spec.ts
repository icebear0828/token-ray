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

function buildSeededPayload(modelCount = 3) {
  const start = new Date('2026-05-01T00:00:00Z');
  const dateAt = (offset: number) => {
    const d = new Date(start);
    d.setUTCDate(start.getUTCDate() + offset);
    return d.toISOString().slice(0, 10);
  };

  const dailyCosts = Array.from({ length: 14 }, (_, idx) => ({
    date: dateAt(idx),
    cost: idx === 0 ? 0 : Number((1.2 + idx * 0.45).toFixed(2)),
    tokens: idx === 0 ? 0 : 1200 + idx * 350,
  }));

  const heatmap = dailyCosts.map((pt) => ({
    date: pt.date,
    tokens: pt.tokens,
    cost: pt.cost,
    events: pt.tokens > 0 ? Math.max(1, Math.round(pt.tokens / 500)) : 0,
  }));

  const models = Array.from({ length: modelCount }, (_, idx) => ({
    model: `model-${String(idx + 1).padStart(2, '0')}`,
    events: 10 + idx,
    input_tokens: 10000 + idx * 500,
    cached_tokens: 4000 + idx * 100,
    cache_creation_tokens: 1000,
    output_tokens: 800 + idx * 40,
    timed_output_tokens: 800 + idx * 40,
    duration_ms: 16000,
    timed_events: 4,
    total_cost: Number((10 - idx * 0.25).toFixed(2)),
    input_price_per_m: 3,
    cached_price_per_m: 0.3,
    cache_creation_price_per_m: 3.75,
    output_price_per_m: 15,
    cache_hit_rate: 40 + (idx % 10),
  }));

  return {
    ...emptyStatsPayload,
    periods: periods.map((p) => ({
      ...p,
      input_tokens: 120000,
      cached_tokens: 72000,
      cache_creation_tokens: 12000,
      output_tokens: 15000,
      timed_output_tokens: 12000,
      duration_ms: 150000,
      timed_events: 18,
      cost: 28.45,
      cache_hit_rate: 60,
    })),
    charts: {
      granularity: 'daily' as const,
      daily_costs: dailyCosts,
      calendar_heatmap: heatmap,
      tool_share: [
        { source: 'Claude Code', tokens: 90000, total_cost: 18.2, events: 24 },
        { source: 'Codex', tokens: 45000, total_cost: 10.25, events: 14 },
      ],
    },
    sources: [
      {
        name: 'Claude Code',
        total_input: 80000,
        total_cached: 50000,
        total_cache_creation: 10000,
        total_output: 10000,
        timed_output_tokens: 8000,
        duration_ms: 100000,
        timed_events: 12,
        total_cost: 18.2,
        total_events: 24,
        cache_hit_rate: 62.5,
        models,
      },
      {
        name: 'Codex',
        total_input: 40000,
        total_cached: 22000,
        total_cache_creation: 2000,
        total_output: 5000,
        timed_output_tokens: 4000,
        duration_ms: 50000,
        timed_events: 6,
        total_cost: 10.25,
        total_events: 14,
        cache_hit_rate: 55,
        models: [],
      },
    ],
    projects: [
      {
        project: 'token-ray',
        events: 38,
        input_tokens: 120000,
        cached_tokens: 72000,
        cache_creation_tokens: 12000,
        output_tokens: 15000,
        timed_output_tokens: 12000,
        duration_ms: 150000,
        timed_events: 18,
        total_cost: 28.45,
        cache_hit_rate: 60,
      },
    ],
  };
}

async function mockBaseRoutes(page: Parameters<typeof test>[0] extends never ? never : import('@playwright/test').Page, payload: unknown) {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, payload);
  });
  await page.route('**/api/sources', async (route) => {
    await fulfillJSON(route, []);
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/system/update/check', async (route) => {
    await fulfillJSON(route, { current_version: 'v1.13.1', latest_version: 'v1.13.1', has_update: false });
  });
}

test('light and dark theme toggle updates html.dark and persists across reload', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await mockBaseRoutes(page, buildSeededPayload());
  await page.goto('/');

  const html = page.locator('html');
  const toggle = page.getByTestId('theme-toggle');

  await expect(toggle).toBeVisible();
  await expect(html).not.toHaveClass(/\bdark\b/);
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');

  await toggle.click();
  await expect(html).toHaveClass(/\bdark\b/);
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  expect(
    await page.evaluate(() => localStorage.getItem('ai-flight-dashboard_theme'))
  ).toBe('dark');

  await page.reload();
  await expect(html).toHaveClass(/\bdark\b/);
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');

  await toggle.click();
  await expect(html).not.toHaveClass(/\bdark\b/);
  expect(
    await page.evaluate(() => localStorage.getItem('ai-flight-dashboard_theme'))
  ).toBe('light');

  expect(pageErrors).toEqual([]);
});

test('DailyCostChart sliding indicator and hover readout track metric and bar hover', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await mockBaseRoutes(page, buildSeededPayload());
  await page.goto('/');

  const slider = page.getByTestId('trend-metric-slider');
  const readout = page.getByTestId('trend-readout');
  const bars = page.getByTestId('daily-cost-bar');

  await expect(slider).toHaveClass(/translate-x-0/);
  await expect(bars.first()).toHaveAttribute('data-cost', '0');
  await expect(bars.first()).toHaveCSS('height', '0px');

  await bars.last().hover();
  await expect(readout).toContainText('2026-05-14');
  await expect(readout).toContainText('$7.05');
  await expect(readout).toContainText('对比均值');

  await page.getByRole('button', { name: 'TOKEN', exact: true }).click();
  await expect(slider).toHaveClass(/translate-x-full/);
  await expect(page.getByTestId('trend-avg-line')).toBeVisible();

  await bars.last().hover();
  await expect(readout).toContainText('5.8K');

  expect(pageErrors).toEqual([]);
});

test('TokenTaxonomyStrip segment hover highlights matching readout card and preserves tok/s', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await mockBaseRoutes(page, buildSeededPayload());
  await page.goto('/');

  const outputSegment = page.getByTestId('taxonomy-segment-output');
  const outputCard = page.getByTestId('taxonomy-card-output');
  const baseInputCard = page.getByTestId('taxonomy-card-base_input');

  await expect(outputCard).toContainText('≈ 80 tok/s');
  await expect(outputCard).toHaveAttribute('data-active', 'false');

  await outputSegment.hover();
  await expect(outputSegment).toHaveAttribute('data-active', 'true');
  await expect(outputCard).toHaveAttribute('data-active', 'true');
  await expect(baseInputCard).toHaveAttribute('data-active', 'false');

  expect(pageErrors).toEqual([]);
});

test('background polling pauses while tab is hidden and refreshes immediately on visible', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await page.clock.install();

  await page.addInitScript(() => {
    let visState: DocumentVisibilityState = 'hidden';
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => visState,
    });
    (window as unknown as { __setVisibility: (state: DocumentVisibilityState) => void }).__setVisibility = (
      state: DocumentVisibilityState
    ) => {
      visState = state;
      document.dispatchEvent(new Event('visibilitychange'));
    };
  });

  let summaryCalls = 0;
  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    if ((url.searchParams.get('detail') ?? 'summary') === 'summary') {
      summaryCalls += 1;
    }
    await fulfillJSON(route, { ...emptyStatsPayload, charts: emptyCharts });
  });
  await page.route('**/api/sources', async (route) => fulfillJSON(route, []));
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/system/update/check', async (route) =>
    fulfillJSON(route, { current_version: 'v1.13.1', latest_version: 'v1.13.1', has_update: false })
  );

  await page.goto('/');
  await expect.poll(() => summaryCalls).toBe(1);

  // Advance virtual clock well past 5s while hidden: polling must remain paused.
  await page.clock.runFor(11000);
  expect(summaryCalls).toBe(1);

  // Returning to visible triggers an immediate background refresh.
  await page.evaluate(() => {
    (window as unknown as { __setVisibility: (state: DocumentVisibilityState) => void }).__setVisibility(
      'visible'
    );
  });
  await expect.poll(() => summaryCalls).toBe(2);

  // Subsequent 5s interval runs while visible.
  await page.clock.runFor(5100);
  await expect.poll(() => summaryCalls).toBeGreaterThanOrEqual(3);

  expect(pageErrors).toEqual([]);
});

test('AttributionTables supports column sorting and PAGE_SIZE=25 pagination', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await mockBaseRoutes(page, buildSeededPayload(30));
  await page.goto('/');

  const rows = page.getByTestId('model-attribution-row');
  await expect(rows).toHaveCount(25);
  await expect(rows.first()).toContainText('model-01');

  await page.getByRole('button', { name: '展开全部 (30)' }).click();
  await expect(rows).toHaveCount(30);

  await page.getByRole('button', { name: '收起至前 25 项' }).click();
  await expect(rows).toHaveCount(25);

  // Click cost column header on model table to toggle ascending order (model-30 has lowest cost).
  await page.getByRole('columnheader', { name: '总成本' }).first().getByRole('button').click();
  await expect(rows.first()).toContainText('model-30');

  expect(pageErrors).toEqual([]);
});

test('layout remains overflow-free across 375, 768, 1024, and 1440px viewports', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await mockBaseRoutes(page, buildSeededPayload(4));

  for (const width of [375, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto('/');
    await expect(page.getByTestId('theme-toggle')).toBeVisible();
    const hasOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    );
    expect(hasOverflow, `horizontal overflow at ${width}px`).toBe(false);
  }

  expect(pageErrors).toEqual([]);
});

test('dark mode preserves body dark surface and drawer control styles without unlayered override', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await mockBaseRoutes(page, buildSeededPayload());
  await page.route('**/api/config', (route) => fulfillConfig(route));
  await page.route('**/api/system/version', (route) =>
    fulfillJSON(route, { version: 'v1.14.1', goos: 'darwin', goarch: 'arm64' })
  );
  await page.route('**/api/system/logs', (route) =>
    fulfillJSON(route, { path: '/tmp/ai-flight-dashboard/stats' })
  );

  await page.goto('/');

  await page.getByTestId('theme-toggle').click();
  await expect(page.locator('html')).toHaveClass(/\bdark\b/);
  await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(9, 13, 22)');

  await page.reload();
  await expect(page.locator('html')).toHaveClass(/\bdark\b/);
  const reloadedBodyBg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(reloadedBodyBg).toBe('rgb(9, 13, 22)');

  await page.getByRole('button', { name: '系统配置' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();

  const checkBtn = dialog.getByRole('button', { name: '检查更新' });
  await expect(checkBtn).toBeVisible();

  const [dialogBg, checkBtnBg] = await Promise.all([
    dialog.evaluate((el) => getComputedStyle(el).backgroundColor),
    checkBtn.evaluate((el) => getComputedStyle(el).backgroundColor),
  ]);
  expect(checkBtnBg).not.toBe(dialogBg);
  expect(checkBtnBg).not.toBe('rgb(15, 23, 42)');

  await checkBtn.hover();
  await expect
    .poll(() => checkBtn.evaluate((el) => getComputedStyle(el).backgroundColor))
    .not.toBe(checkBtnBg);

  expect(pageErrors).toEqual([]);
});
