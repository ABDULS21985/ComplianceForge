import { test, expect, type Page } from '@playwright/test';

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel(/email/i).fill('admin@complianceforge.io');
  await page.getByLabel(/password/i).fill('Admin123!@#');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/dashboard/);
}

test.describe('Incidents', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('incident list loads', async ({ page }) => {
    await page.goto('/incidents');

    await expect(page.getByRole('heading', { name: /incident management/i })).toBeVisible();
    await expect(page.locator('table').first()).toBeVisible({ timeout: 15000 });
  });

  test('can open and submit report incident form', async ({ page }) => {
    await page.goto('/incidents');

    await page.getByRole('button', { name: /report incident/i }).click();
    const container = page.getByRole('dialog').first();
    const fallbackContainer = page.locator('form').first();
    if (await container.isVisible()) {
      await expect(container).toBeVisible();
    } else {
      await expect(fallbackContainer).toBeVisible();
    }

    const titleInput = page.locator('#inc-title, input[name="title"]').first();
    await titleInput.fill('E2E Incident - Data Exposure');

    const descInput = page.locator('#inc-desc, textarea[name="description"]').first();
    await descInput.fill('Automated E2E incident creation test case.');

    const typeInput = page.locator('#inc-type, input[name="incident_type"]').first();
    await typeInput.fill('Data Exposure');

    const categoryInput = page.locator('#inc-cat, input[name="category"]').first();
    await categoryInput.fill('Security');

    const severityTrigger = page.locator('#inc-severity').locator('..').locator('button').first();
    if (await severityTrigger.isVisible()) {
      await severityTrigger.click();
      await page.getByRole('option', { name: /medium|high|low|critical/i }).first().click();
    }

    await page.getByRole('button', { name: /submit report|save|create/i }).click();

    await expect(page.getByText(/incident reported|success|created/i).first()).toBeVisible({ timeout: 15000 });
  });

  test('urgent breach section supports notify dpa action when present', async ({ page }) => {
    await page.goto('/incidents');

    const notifyButtons = page.getByRole('button', { name: /notify dpa/i });
    const count = await notifyButtons.count();
    if (count > 0) {
      await notifyButtons.first().click();
      await expect(page.getByText(/notified|success|recorded/i).first()).toBeVisible({ timeout: 15000 });
    }
  });
});
