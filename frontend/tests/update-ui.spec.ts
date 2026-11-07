import { expect, test, type Page } from '@playwright/test';

import {
  emptyStatsPayload,
  fulfillEmptyLANScan,
  fulfillJSON,
  fulfillLANStatus,
} from './dashboard-helpers';

const updateBanner = (page: Page) => page.locator('[aria-label="update-notification"]');

async function mockDashboard(page: Page) {
  await page.route('**/api/stats?*', (route) => fulfillJSON(route, emptyStatsPayload));
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/pricing', (route) => fulfillJSON(route, []));
  await page.route('**/api/devices**', (route) => fulfillJSON(route, []));
  await page.route('**/api/config', (route) =>
    fulfillJSON(route, {
      auto_start: false,
      extra_watch_dirs: [],
      enable_lan: true,
      extra_peer_hosts: [],
      tailscale_discovery: true,
    })
  );
}

async function mockVersion(page: Page) {
  await page.route('**/api/system/version', (route) =>
    fulfillJSON(route, { version: 'v1.2.3', goos: 'linux', goarch: 'amd64' })
  );
}

async function openSystemTab(page: Page) {
  await page.getByRole('button', { name: /系统配置|Scanner Config/ }).first().click();
}

test('settings shows current version and reports up to date', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await mockDashboard(page);
  await mockVersion(page);
  await page.route('**/api/system/update/check', (route) =>
    fulfillJSON(route, {
      current_version: 'v1.2.3',
      latest_version: 'v1.2.3',
      has_update: false,
    })
  );

  await page.goto('/');
  await openSystemTab(page);

  await expect(page.getByText('v1.2.3', { exact: true })).toBeVisible();
  await expect(page.getByText(/linux\/amd64/)).toBeVisible();

  await page.getByRole('button', { name: /检查更新|CHECK FOR UPDATES/ }).click();
  await expect(page.getByText(/当前已是最新版本|You are on the latest version/)).toBeVisible();

  expect(pageErrors).toEqual([]);
});

test('settings shows an available update, release notes, and applies it', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await mockDashboard(page);
  await mockVersion(page);
  await page.route('**/api/system/update/check', (route) =>
    fulfillJSON(route, {
      current_version: 'v1.2.3',
      latest_version: 'v1.3.0',
      has_update: true,
      release_notes: '## Changes\n- Faster updates',
      asset_name: 'ai-flight-dashboard-linux.tar.gz',
      download_url: 'https://example.com/asset',
    })
  );

  let applyCalls = 0;
  await page.route('**/api/system/update/apply', async (route) => {
    applyCalls += 1;
    await fulfillJSON(route, {
      success: true,
      message: 'Successfully updated to v1.3.0. Please restart.',
      version: 'v1.3.0',
    });
  });

  await page.goto('/');

  // The mount-time check shows the banner; dismiss it so the settings panel owns the UI.
  await expect(updateBanner(page)).toBeVisible();
  await updateBanner(page).getByRole('button', { name: 'Dismiss' }).click();
  await expect(updateBanner(page)).toHaveCount(0);

  await openSystemTab(page);
  await page.getByRole('button', { name: /检查更新|CHECK FOR UPDATES/ }).click();

  await expect(page.getByText(/发现新版本|New Version Available/)).toBeVisible();
  await expect(page.getByText('v1.3.0', { exact: true })).toBeVisible();
  await expect(page.getByText(/Faster updates/)).toBeVisible();
  await expect(page.getByText(/ai-flight-dashboard-linux\.tar\.gz/)).toBeVisible();

  await page.getByRole('button', { name: /立即更新|APPLY UPDATE/ }).click();
  await expect(page.getByText(/Successfully updated to v1\.3\.0/)).toBeVisible();
  expect(applyCalls).toBe(1);

  expect(pageErrors).toEqual([]);
});

test('update check failure is surfaced instead of failing silently', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await mockDashboard(page);
  await mockVersion(page);
  await page.route('**/api/system/update/check', (route) =>
    fulfillJSON(route, {
      current_version: 'v1.2.3',
      latest_version: 'v1.2.3',
      has_update: false,
      error: 'GitHub API rate limit exceeded',
    })
  );

  await page.goto('/');
  await openSystemTab(page);
  await page.getByRole('button', { name: /检查更新|CHECK FOR UPDATES/ }).click();

  await expect(page.getByText(/GitHub API rate limit exceeded/)).toBeVisible();
  await expect(page.getByText(/当前已是最新版本|You are on the latest version/)).toHaveCount(0);

  expect(pageErrors).toEqual([]);
});

test('apply failure shows the server error message', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await mockDashboard(page);
  await mockVersion(page);
  await page.route('**/api/system/update/check', (route) =>
    fulfillJSON(route, {
      current_version: 'v1.2.3',
      latest_version: 'v1.3.0',
      has_update: true,
      asset_name: 'ai-flight-dashboard-linux.tar.gz',
    })
  );
  await page.route('**/api/system/update/apply', (route) =>
    fulfillJSON(
      route,
      { success: false, message: 'Failed to apply update: permission denied' },
      500
    )
  );

  await page.goto('/');
  await expect(updateBanner(page)).toBeVisible();
  await updateBanner(page).getByRole('button', { name: 'Dismiss' }).click();

  await openSystemTab(page);
  await page.getByRole('button', { name: /检查更新|CHECK FOR UPDATES/ }).click();
  await page.getByRole('button', { name: /立即更新|APPLY UPDATE/ }).click();

  await expect(page.getByText(/permission denied/)).toBeVisible();

  expect(pageErrors).toEqual([]);
});

test('header banner announces a new version and opens settings', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await mockDashboard(page);
  await mockVersion(page);
  await page.route('**/api/system/update/check', (route) =>
    fulfillJSON(route, {
      current_version: 'v1.2.3',
      latest_version: 'v1.3.0',
      has_update: true,
      asset_name: 'ai-flight-dashboard-linux.tar.gz',
    })
  );

  await page.goto('/');

  await expect(updateBanner(page)).toBeVisible();
  await expect(updateBanner(page)).toContainText('v1.3.0');

  await updateBanner(page).getByRole('button', { name: /查看详情|View Details/ }).click();
  await expect(page.getByRole('heading', { name: /设置面板|SETTINGS PANEL/ })).toBeVisible();

  expect(pageErrors).toEqual([]);
});
