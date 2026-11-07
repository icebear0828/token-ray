import { expect, test } from '@playwright/test';

import {
  emptyStatsPayload,
  fulfillConfig,
  fulfillEmptyLANScan,
  fulfillJSON,
  fulfillLANStatus,
  fulfillLANSelf,
} from './dashboard-helpers';

test('device drawer can create edit clear and soft-delete devices', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  await page.setViewportSize({ width: 390, height: 820 });

  type DeviceRow = {
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
  };

  let devices: DeviceRow[] = [{
    id: 'nas.local',
    display_name: 'nas.local',
    events: 12,
    input_tokens: 1000,
    cached_tokens: 100,
    cache_creation_tokens: 50,
    output_tokens: 25,
    total_cost: 3.5,
    first_seen: '2026-05-01T12:00:00Z',
    last_seen: '2026-05-10T12:00:00Z',
  }];
  let clearedAlias = '';
  let softDeletedDevice = '';

  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, emptyStatsPayload);
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/lan/self', (route) => fulfillLANSelf(route, 'local-machine'));
  await page.route('**/api/pricing', async (route) => {
    await fulfillJSON(route, []);
  });
  await page.route('**/api/config', (route) => fulfillConfig(route));
  await page.route('**/api/system/logs', async (route) => {
    await fulfillJSON(route, { path: '/tmp/ai-flight-dashboard/stats' });
  });
  await page.route('**/api/devices**', async (route) => {
    if (route.request().method() === 'DELETE') {
      const url = new URL(route.request().url());
      softDeletedDevice = url.searchParams.get('device_id') ?? '';
      devices = devices.filter((device) => device.id !== softDeletedDevice);
      await fulfillJSON(route, { device_id: softDeletedDevice, superseded_count: 1 });
      return;
    }
    await fulfillJSON(route, devices);
  });
  await page.route('**/api/device-alias**', async (route) => {
    if (route.request().method() === 'POST') {
      const body = await route.request().postDataJSON() as { device_id: string; display_name: string };
      const existing = devices.find((device) => device.id === body.device_id);
      if (existing) {
        existing.display_name = body.display_name;
      } else {
        devices = [{
          id: body.device_id,
          display_name: body.display_name,
          events: 0,
          input_tokens: 0,
          cached_tokens: 0,
          cache_creation_tokens: 0,
          output_tokens: 0,
          total_cost: 0,
        }, ...devices];
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: '' });
      return;
    }
    if (route.request().method() === 'DELETE') {
      const url = new URL(route.request().url());
      clearedAlias = url.searchParams.get('device_id') ?? '';
      devices = devices.map((device) =>
        device.id === clearedAlias ? { ...device, display_name: device.id } : device
      );
      await route.fulfill({ status: 200, contentType: 'application/json', body: '' });
      return;
    }
    await route.fulfill({ status: 405, contentType: 'text/plain', body: 'Method not allowed' });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '设备管理' }).click();

  await expect(page.getByTestId('device-row').filter({ hasText: 'nas.local' })).toBeVisible();

  await page.getByPlaceholder('设备 ID').fill('probe-local');
  await page.getByPlaceholder('显示名称').fill('Probe Local');
  await page.getByRole('button', { name: '添加别名' }).click();
  await expect(page.getByText('Probe Local', { exact: true })).toBeVisible();

  const nasRow = page.getByTestId('device-row').filter({ hasText: 'nas.local' });
  await nasRow.getByPlaceholder('输入设备别名...').fill('NAS');
  await nasRow.getByRole('button', { name: '保存' }).click();
  await expect(page.getByText('NAS', { exact: true })).toBeVisible();

  await nasRow.getByRole('button', { name: '清除别名' }).click();
  await expect.poll(() => clearedAlias).toBe('nas.local');

  const probeRow = page.getByTestId('device-row').filter({ hasText: 'probe-local' });
  await probeRow.getByRole('button', { name: '软删除设备' }).click();
  await page.getByRole('button', { name: '确认软删除' }).click();
  await expect.poll(() => softDeletedDevice).toBe('probe-local');
  await expect(page.getByText('probe-local', { exact: true })).toHaveCount(0);
  await expect(page.getByTestId('device-management-list')).toBeVisible();
  expect(
    await page
      .getByTestId('device-management-list')
      .evaluate((element) => element.scrollWidth <= element.clientWidth)
  ).toBe(true);
  expect(pageErrors).toEqual([]);
});

test('config drawer can add an extra watch directory when backend returns null dirs', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  await page.setViewportSize({ width: 390, height: 820 });

  type ConfigPayload = {
    auto_start: boolean;
    extra_watch_dirs: string[];
    enable_lan?: boolean;
  };

  let savedConfig: ConfigPayload | null = null;

  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, emptyStatsPayload);
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/pricing', async (route) => {
    await fulfillJSON(route, []);
  });
  await page.route('**/api/devices**', async (route) => {
    await fulfillJSON(route, []);
  });
  await page.route('**/api/system/logs', async (route) => {
    await fulfillJSON(route, { path: '/tmp/ai-flight-dashboard/stats' });
  });
  await page.route('**/api/config', async (route) => {
    if (route.request().method() === 'PUT') {
      savedConfig = await route.request().postDataJSON() as ConfigPayload;
      await fulfillJSON(route, savedConfig);
      return;
    }
    await fulfillJSON(route, {
      auto_start: false,
      extra_watch_dirs: null,
      enable_lan: true,
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '系统配置' }).click();

  await page.getByPlaceholder('输入绝对路径...').fill('/tmp/token-ray-extra');
  await page.getByRole('button', { name: '添加路径' }).click();
  await expect(page.getByText('/tmp/token-ray-extra', { exact: true })).toBeVisible();

  await page.getByRole('button', { name: '保存配置' }).click();

  await expect.poll(() => savedConfig?.extra_watch_dirs).toEqual(['/tmp/token-ray-extra']);
  expect(pageErrors).toEqual([]);
});
