import { expect, type Page, test } from '@playwright/test';
import { SUPPORT_TEST_TENANT, supportBundleFixture } from '../src/test/support-bundle-fixture';
import type { AxeResults } from 'axe-core';
import type { DiagnosticsSnapshot } from '../src/types/diagnostics';
import { join } from 'node:path';
import { readFileSync } from 'node:fs';
import type { User } from '../src/types';

const axeScript = readFileSync(join(process.cwd(), 'node_modules/axe-core/axe.min.js'), 'utf8');

function snapshot(): DiagnosticsSnapshot {
  return {
    organization_id: SUPPORT_TEST_TENANT, generated_at: new Date().toISOString(), overall_status: 'healthy',
    dependencies: [], configuration: [],
    migration: { current_version: 58, supported_version: 58, pending_count: 0, dirty: false, status: 'healthy' },
    queue: { pending: 0, leased: 0, dead: 0, expired_leases: 0, oldest_ready_age_seconds: 0, inbox_processing: 0, inbox_expired_leases: 0, status: 'healthy' },
    notifications: { due: 0, expired_leases: 0, terminal_failures: 0, oldest_due_age_seconds: 0, status: 'healthy' },
    connectors: { total: 0, healthy: 0, degraded: 0, unhealthy: 0, unknown: 0, recent_failed_syncs: 0, status: 'healthy' },
  };
}

async function authenticatedDiagnostics(page: Page, configure = true) {
  const user: User = {
    id: '1dc70457-0207-462a-83af-29aeea0e7a6f', organization_id: SUPPORT_TEST_TENANT,
    email: 'reviewer@example.test', first_name: 'Support', last_name: 'Reviewer', status: 'active',
    is_super_admin: false, language: 'en', created_at: '2026-09-15T07:00:00Z', updated_at: '2026-09-15T07:00:00Z',
  };
  const origin = new URL(test.info().project.use.baseURL as string);
  // Chromium's cookie injection validates __Host- cookies against an HTTPS URL.
  // Loopback remains a trustworthy HTTP test origin; production flags stay intact.
  origin.protocol = 'https:';
  await page.context().addCookies([{
    name: '__Host-cf_access_token', value: 'http-only-production-browser-fixture', url: origin.href,
    secure: true, httpOnly: true, sameSite: 'Lax',
  }]);
  await page.route('**/api/auth/session', (route) => route.fulfill({ json: user }));
  await page.route('**/api/auth/csrf', (route) => route.fulfill({ json: { csrf_token: 'browser-csrf-proof' } }));
  await page.route('**/api/bff/access/my-permissions', (route) => route.fulfill({
    json: { data: { settings: configure ? ['read', 'configure'] : ['read'] } },
  }));
  await page.route('**/api/bff/settings/capabilities/data_lifecycle/evaluation', (route) => route.fulfill({ json: { enabled: false } }));
  await page.route('**/api/bff/notifications/unread-count', (route) => route.fulfill({ json: { count: 0 } }));
  await page.route('**/api/bff/settings/diagnostics', (route) => route.fulfill({ json: { data: snapshot() } }));
  await page.goto('/settings/diagnostics');
  await expect(page.getByRole('heading', { name: 'Administrator diagnostics', exact: true })).toBeVisible();
}

test('production support preview and verified local download work by keyboard without persisting credentials', async ({ page }) => {
  let generations = 0;
  const fixture = supportBundleFixture();
  await page.route('**/api/bff/settings/diagnostics/support-bundle', async (route) => {
    generations += 1;
    expect(route.request().method()).toBe('POST');
    expect(route.request().postDataJSON()).toEqual({ consent: true, scope: 'health_and_posture' });
    expect(route.request().headers()['x-cf-csrf-token']).toBe('browser-csrf-proof');
    expect(route.request().headers().authorization).toBeUndefined();
    await route.fulfill({ status: 200, headers: fixture.headers, body: Buffer.from(fixture.bytes) });
  });
  await authenticatedDiagnostics(page);
  const trigger = page.getByRole('button', { name: 'Preview support bundle' });
  await trigger.focus(); await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Review and consent to a support bundle' });
  await expect(dialog.getByRole('heading', { name: 'Included operational metadata' })).toBeFocused();
  await expect(dialog.getByRole('button', { name: 'Generate bundle' })).toBeDisabled();
  expect(generations).toBe(0);
  await page.setViewportSize({ width: 320, height: 800 });
  await page.addScriptTag({ content: axeScript });
  const results = await page.evaluate(async () => (window as unknown as {
    axe: { run: (context: Element, options: object) => Promise<AxeResults> };
  }).axe.run(document.querySelector('[role="dialog"]')!, {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'] },
  }));
  expect(results.violations.map(({ id }) => id)).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1)).toBe(true);
  await page.keyboard.press('Tab');
  await expect(dialog.getByRole('checkbox')).toBeFocused();
  await page.keyboard.press('Space'); await page.keyboard.press('Tab'); await page.keyboard.press('Tab');
  await expect(dialog.getByRole('button', { name: 'Generate bundle' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(dialog.getByRole('heading', { name: 'Bundle integrity checked' })).toBeFocused();
  expect(generations).toBe(1);
  const downloaded = page.waitForEvent('download');
  await dialog.getByRole('button', { name: 'Download local ZIP' }).click();
  expect((await downloaded).suggestedFilename()).toBe(`complianceforge-support-${fixture.manifest.bundle_id}.zip`);
  await expect(dialog.getByRole('status')).toContainText('successful receipt has not been confirmed');
  await page.keyboard.press('Escape'); await expect(trigger).toBeFocused();
  await expect(page.locator('a[href^="blob:"]')).toHaveCount(0);
  const browserStorage = await page.evaluate(() => ({ cookie: document.cookie, local: { ...localStorage }, session: { ...sessionStorage } }));
  expect(JSON.stringify(browserStorage)).not.toContain('http-only-production-browser-fixture');
  expect(JSON.stringify(browserStorage)).not.toMatch(/access_token|refresh_token/);
});

test('production support errors are safe and retries require a new explicit consent', async ({ page }) => {
  let generations = 0;
  await page.route('**/api/bff/settings/diagnostics/support-bundle', (route) => {
    generations += 1;
    return route.fulfill({ status: 503, json: { error_code: 'INTERNAL_ERROR', message: 'password=private.internal' } });
  });
  await authenticatedDiagnostics(page);
  await page.getByRole('button', { name: 'Preview support bundle' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('checkbox').check(); await dialog.getByRole('button', { name: 'Generate bundle' }).click();
  await expect(dialog.getByRole('alert')).toBeFocused();
  await expect(dialog.getByRole('alert')).not.toContainText('private.internal');
  await expect(dialog.getByRole('checkbox')).not.toBeChecked();
  await expect(dialog.getByRole('button', { name: 'Generate bundle' })).toBeDisabled();
  expect(generations).toBe(1);
  await dialog.getByRole('checkbox').check(); await dialog.getByRole('button', { name: 'Generate bundle' }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  expect(generations).toBe(2);
});

test('production diagnostics read-only access cannot preview or generate a support bundle', async ({ page }) => {
  let generations = 0;
  await page.route('**/api/bff/settings/diagnostics/support-bundle', (route) => {
    generations += 1; return route.fulfill({ status: 403 });
  });
  await authenticatedDiagnostics(page, false);
  await expect(page.getByText('Generation requires verified settings:configure access for this tenant.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Preview support bundle' })).toHaveCount(0);
  expect(generations).toBe(0);
});

test('production support transport never replays a consent POST at an unexpected redirect receiver', async ({ page }) => {
  let generations = 0;
  let receiverRequests = 0;
  const receiver = new URL('/unexpected-support-receiver', test.info().project.use.baseURL as string).href;
  // Same-origin keeps CSP from masking whether fetch's redirect policy works.
  await page.route(receiver, (route) => {
    receiverRequests += 1; return route.fulfill({ status: 500 });
  });
  // Defense-in-depth fixture: the real BFF also rejects this upstream status.
  await page.route('**/api/bff/settings/diagnostics/support-bundle', (route) => {
    generations += 1;
    return route.fulfill({ status: 307, headers: { location: receiver } });
  });
  await authenticatedDiagnostics(page);
  await page.getByRole('button', { name: 'Preview support bundle' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('checkbox').check(); await dialog.getByRole('button', { name: 'Generate bundle' }).click();
  await expect(dialog.getByRole('alert')).toBeFocused();
  await expect(dialog.getByRole('checkbox')).not.toBeChecked();
  await expect(dialog.getByRole('button', { name: 'Generate bundle' })).toBeDisabled();
  expect(generations).toBe(1);
  expect(receiverRequests).toBe(0);
});
