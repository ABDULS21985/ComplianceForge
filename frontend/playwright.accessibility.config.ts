import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.PLAYWRIGHT_ACCESSIBILITY_BASE_URL || 'http://127.0.0.1:3100';

export default defineConfig({
  testDir: './e2e',
  testMatch: ['accessibility.spec.ts', 'support-bundles.spec.ts'],
  workers: 1,
  retries: 0,
  timeout: 30_000,
  reporter: [['list']],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: process.env.PLAYWRIGHT_SKIP_WEBSERVER === '1'
    ? undefined
    : {
        command: 'node scripts/start-accessibility-server.mjs',
        url: baseURL,
        reuseExistingServer: true,
        timeout: 60_000,
      },
});
