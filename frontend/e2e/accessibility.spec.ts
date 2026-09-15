import { expect, test } from '@playwright/test';
import type { AxeResults } from 'axe-core';
import { join } from 'node:path';
import { readFileSync } from 'node:fs';

const axeScript = readFileSync(join(process.cwd(), 'node_modules/axe-core/axe.min.js'), 'utf8');
const publicRoutes = [
  '/login',
  '/forgot-password',
  '/reset-password',
  '/verify-email',
  '/accept-invitation',
  '/board-portal',
  '/vendor-portal',
];

for (const route of publicRoutes) {
  test(`${route} has an axe-clean public shell and reflows at 320 CSS pixels`, async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 800 });
    await page.goto(route);
    await expect(page.getByRole('main')).toHaveCount(1);
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
    if (route === '/board-portal' || route === '/vendor-portal') {
      await expect(page.getByRole('heading', {
        name: route === '/board-portal' ? 'Board portal unavailable' : 'Vendor assessment unavailable',
      })).toBeVisible();
    }
    await expect(page).not.toHaveTitle('ComplianceForge');

    await page.addScriptTag({ content: axeScript });
    const results = await page.evaluate(async () => {
      const audit = (window as unknown as {
        axe: { run: (context: Document, options: object) => Promise<AxeResults> };
      }).axe;
      return audit.run(document, {
        runOnly: {
          type: 'tag',
          values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'],
        },
      });
    });
    expect(results.violations.map((violation) => ({
      id: violation.id,
      nodes: violation.nodes.map((node) => node.target),
    }))).toEqual([]);

    const width = await page.evaluate(() => ({
      content: document.documentElement.scrollWidth,
      viewport: document.documentElement.clientWidth,
    }));
    expect(width.content).toBeLessThanOrEqual(width.viewport + 1);
    const targets = await page.locator('button:visible').evaluateAll((buttons) =>
      buttons.map((button) => button.getBoundingClientRect().height),
    );
    for (const height of targets) expect(height).toBeGreaterThanOrEqual(44);
  });
}

test('client auth navigation restores route focus and reduced motion suppresses transitions', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/login');
  const signIn = page.getByRole('button', { name: 'Sign in with password' });
  await expect(signIn).toBeVisible();
  const transition = await signIn.evaluate((element) =>
    getComputedStyle(element).transitionDuration.split(',').map((value) => Number.parseFloat(value)),
  );
  expect(Math.max(...transition)).toBeLessThanOrEqual(0.005);

  await page.getByRole('link', { name: 'Forgot password?' }).click();
  const heading = page.getByRole('heading', { name: 'Reset your password' });
  await expect(heading).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('textbox', { name: 'Organization ID' })).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('textbox', { name: 'Email address' })).toBeFocused();
});

test('offline connectivity is announced without obscuring the account form', async ({ page, context }) => {
  await page.goto('/login');
  await context.setOffline(true);
  const alert = page.getByRole('alert').filter({ hasText: 'You are offline' });
  await expect(alert).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign in with password' })).toBeVisible();
  await context.setOffline(false);
  await expect(alert).toHaveCount(0);
});
