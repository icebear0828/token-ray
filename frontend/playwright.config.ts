import { defineConfig, devices } from '@playwright/test';

const port = process.env.PW_PORT ?? '19183';
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  expect: {
    timeout: 5000,
  },
  use: {
    baseURL,
    trace: 'on-first-retry',
  },
  webServer: {
    command:
      process.env.PW_WEBSERVER_COMMAND ??
      `node ./tests/start-real-dashboard.mjs --port ${port}`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 60000,
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        channel: process.env.PW_CHANNEL,
      },
    },
  ],
});
