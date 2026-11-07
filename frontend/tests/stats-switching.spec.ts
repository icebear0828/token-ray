import { expect, test, type Route } from '@playwright/test';

import {
  detailsPayload,
  emptyCharts,
  emptyStatsPayload,
  fulfillEmptyLANScan,
  fulfillJSON,
  fulfillLANStatus,
  periods,
} from './dashboard-helpers';

const summaryPayload = (source: string, cost = 2.5) => ({
  periods,
  charts: emptyCharts,
  sources: source
    ? [{
        name: source,
        total_input: 1000,
        total_cached: 100,
        total_cache_creation: 50,
        total_output: 25,
        total_cost: cost,
        total_events: 1,
        cache_hit_rate: 10,
        models: [],
      }]
    : [],
  devices: [{ id: 'local', display_name: 'local' }],
  projects: [],
  is_paused: false,
});

test('source switching ignores stale stats responses', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  let heldClaudeRoute: Route | null = null;

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    const detail = url.searchParams.get('detail') ?? 'summary';

    if (source === 'Claude Code') {
      heldClaudeRoute = route;
      return;
    }

    if (detail === 'details' && source) {
      await fulfillJSON(route, detailsPayload(source, source === 'Codex' ? 'gpt-5.5' : 'claude-opus-4-7'));
      return;
    }

    await fulfillJSON(route, summaryPayload(source));
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Claude Code' })).toBeVisible();

  await page.getByRole('button', { name: 'Claude Code' }).click();
  await page.getByRole('button', { name: 'Codex' }).click();

  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
  expect(heldClaudeRoute).not.toBeNull();

  await heldClaudeRoute?.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(summaryPayload('Claude Code', 1.5)),
  });

  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
  await expect(page.getByText('claude-opus-4-7', { exact: true })).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});

test('source switching includes Antigravity source tab', async ({ page }) => {
  const requestedSources: string[] = [];

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    const detail = url.searchParams.get('detail') ?? 'summary';
    requestedSources.push(source);

    if (detail === 'details' && source === 'Antigravity') {
      await fulfillJSON(route, {
        ...detailsPayload('Antigravity', 'gemini-3.5-flash', 0.003765),
        periods: [],
        charts: emptyCharts,
      });
      return;
    }

    await fulfillJSON(route, {
      ...summaryPayload(source, source === 'Antigravity' ? 0.003765 : 2.5),
      periods: periods.map((period) => ({
        ...period,
        input_tokens: source === 'Antigravity' ? 2150 : 0,
        cached_tokens: source === 'Antigravity' ? 100 : 0,
        output_tokens: source === 'Antigravity' ? 75 : 0,
        cost: source === 'Antigravity' ? 0.003765 : 0,
        cache_hit_rate: source === 'Antigravity' ? 4.651162790697675 : 0,
      })),
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Antigravity' })).toBeVisible();

  await page.getByRole('button', { name: 'Antigravity' }).click();
  await expect(page.getByText('gemini-3.5-flash', { exact: true })).toBeVisible();
  await expect(page.getByText('$0.0038').first()).toBeVisible();
  expect(requestedSources).toContain('Antigravity');
});

test('source switching renders summary before delayed details', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  let heldCodexDetailsRoute: Route | null = null;
  const requestedDetails: string[] = [];

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    const detail = url.searchParams.get('detail') ?? 'summary';
    requestedDetails.push(detail);

    if (detail === 'details' && source === 'Codex') {
      heldCodexDetailsRoute = route;
      return;
    }

    if (detail === 'details') {
      if (!source) {
        await fulfillJSON(route, { ...emptyStatsPayload, sources: [], projects: [] });
        return;
      }
      await fulfillJSON(route, detailsPayload(source, 'gpt-5.5'));
      return;
    }

    await fulfillJSON(route, summaryPayload(source));
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Codex' })).toBeVisible();

  await page.getByRole('button', { name: 'Codex' }).click();
  await expect(page.getByText('gpt-5.5', { exact: true })).toHaveCount(0);
  await expect(page.getByText('token', { exact: true })).toHaveCount(0);
  await expect.poll(() => heldCodexDetailsRoute).not.toBeNull();
  expect(requestedDetails).toContain('summary');
  expect(requestedDetails).toContain('details');

  await heldCodexDetailsRoute?.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(detailsPayload('Codex', 'gpt-5.5')),
  });

  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
  await expect(page.getByText('token', { exact: true })).toBeVisible();
  expect(pageErrors).toEqual([]);
});

test('background refresh keeps project stats visible while details reload', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  let detailsRequests = 0;
  let summaryRequests = 0;
  let heldRefreshDetailsRoute: Route | null = null;

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const detail = url.searchParams.get('detail') ?? 'summary';

    if (detail === 'details') {
      detailsRequests += 1;
      if (detailsRequests === 2) {
        heldRefreshDetailsRoute = route;
        return;
      }
      await fulfillJSON(route, detailsPayload('Codex', 'gpt-5.5'));
      return;
    }

    summaryRequests += 1;
    await fulfillJSON(route, summaryPayload('Codex'));
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();

  await expect.poll(() => heldRefreshDetailsRoute !== null, { timeout: 7000 }).toBe(true);
  expect(summaryRequests).toBeGreaterThanOrEqual(2);
  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();

  await heldRefreshDetailsRoute?.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(detailsPayload('Codex', 'gpt-5.5')),
  });

  expect(pageErrors).toEqual([]);
});

test('source switching keeps summary visible when details fail', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    const detail = url.searchParams.get('detail') ?? 'summary';
    if (detail === 'details' && source === 'Codex') {
      await route.fulfill({
        status: 500,
        contentType: 'text/plain',
        body: 'details unavailable',
      });
      return;
    }

    await fulfillJSON(route, summaryPayload(source));
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Codex' }).first()).toBeVisible();

  await page.getByRole('button', { name: 'Codex' }).first().click();
  await expect(page.getByRole('button', { name: 'Codex' }).first()).toBeVisible();
  await expect(page.getByText('刷新失败').first()).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});
