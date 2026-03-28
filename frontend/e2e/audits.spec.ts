import { test, expect, type Page } from '@playwright/test';

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel(/email/i).fill('admin@complianceforge.io');
  await page.getByLabel(/password/i).fill('Admin123!@#');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/dashboard/);
}

test.describe('Audits', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('audit list loads', async ({ page }) => {
    await page.goto('/audits');

    await expect(page.getByRole('heading', { name: /audit management/i })).toBeVisible();
    await expect(page.locator('table').first()).toBeVisible({ timeout: 15000 });
  });

  test('can open plan audit form and submit basic fields', async ({ page }) => {
    await page.goto('/audits');

    await page.getByRole('button', { name: /plan audit/i }).click();
    await expect(page.getByRole('dialog')).toBeVisible();

    await page.getByLabel(/title/i).fill('E2E Annual Internal Audit');
    await page.getByLabel(/description/i).fill('E2E generated audit record.');

    await page.getByRole('button', { name: /create audit|save|submit/i }).click();
  });
});
