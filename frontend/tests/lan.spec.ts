import { expect, test } from '@playwright/test';

import { emptyStatsPayload, fulfillJSON } from './dashboard-helpers';

test('LAN radar shows per-source peer totals before full sync completes', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, emptyStatsPayload);
  });
  await page.route('**/api/lan/scan', async (route) => {
    await fulfillJSON(route, {
      peers: ['icebear'],
      peer_infos: [{
        id: 'icebear',
        display_name: 'icebear',
        ip: '192.168.10.5',
        http_port: 19100,
        sync_status: 'syncing',
        tokens_24h: 6000,
        tokens_total: 13000,
        cost_total: 12.34,
        sources: [
          { source: 'Claude Code', tokens_24h: 1000, tokens_total: 2000, cost_total: 2.00 },
          { source: 'Codex', tokens_24h: 2000, tokens_total: 4000, cost_total: 4.00 },
          { source: 'Gemini CLI', tokens_24h: 3000, tokens_total: 7000, cost_total: 6.34 },
        ],
      }],
    });
  });
  await page.route('**/api/lan/status', async (route) => {
    await fulfillJSON(route, { enabled: true });
  });

  await page.goto('/');
  await page.getByRole('button', { name: 'LAN 雷达' }).click();

  const drawer = page.locator('aside');
  await expect(drawer.getByText('Claude Code')).toBeVisible();
  await expect(drawer.getByText('Codex', { exact: true })).toBeVisible();
  await expect(drawer.getByText('Gemini CLI')).toBeVisible();
  await expect(drawer.getByText('2.0K', { exact: true })).toBeVisible();
  await expect(drawer.getByText('4.0K', { exact: true })).toBeVisible();
  await expect(drawer.getByText('7.0K', { exact: true })).toBeVisible();
});

test('LAN radar can leave and rejoin the network', async ({ page }) => {
  let lanEnabled = false;
  const lanActions: string[] = [];

  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, emptyStatsPayload);
  });
  await page.route('**/api/lan/status', async (route) => {
    await fulfillJSON(route, { enabled: lanEnabled });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await fulfillJSON(
      route,
      lanEnabled
        ? { peers: ['peer-a'], peer_infos: [{ id: 'peer-a', display_name: 'peer-a' }] }
        : { peers: [], peer_infos: [] }
    );
  });
  await page.route('**/api/lan/join', async (route) => {
    lanActions.push('join');
    lanEnabled = true;
    await fulfillJSON(route, { enabled: true });
  });
  await page.route('**/api/lan/leave', async (route) => {
    lanActions.push('leave');
    lanEnabled = false;
    await fulfillJSON(route, { enabled: false });
  });

  await page.goto('/');
  await page.getByRole('button', { name: 'LAN 雷达' }).click();

  await expect(page.getByRole('button', { name: '加入局域网发现' })).toBeVisible();
  await page.getByRole('button', { name: '加入局域网发现' }).click();
  await expect(page.getByRole('button', { name: '退出局域网' })).toBeVisible();

  await page.getByRole('button', { name: '退出局域网' }).click();
  await expect(page.getByRole('button', { name: '加入局域网发现' })).toBeVisible();

  await page.getByRole('button', { name: '加入局域网发现' }).click();
  await expect(page.getByRole('button', { name: '退出局域网' })).toBeVisible();
  expect(lanActions).toEqual(['join', 'leave', 'join']);
});
