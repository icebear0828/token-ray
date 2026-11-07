import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  expect: {
    timeout: 5000,
  },
  use: {
    baseURL: 'http://127.0.0.1:19183',
    trace: 'on-first-retry',
  },
  webServer: {
    command: process.env.PW_WEBSERVER_COMMAND ?? 'node ./tests/start-real-dashboard.mjs',
    url: 'http://127.0.0.1:19183',
    reuseExistingServer: !process.env.CI,
    timeout: 60000,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
