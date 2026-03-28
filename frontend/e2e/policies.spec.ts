import { test, expect, type Page } from '@playwright/test';

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel(/email/i).fill('admin@complianceforge.io');
  await page.getByLabel(/password/i).fill('Admin123!@#');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/dashboard/);
}

test.describe('Policies', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('policy list loads', async ({ page }) => {
    await page.goto('/policies');

    await expect(page.getByRole('heading', { name: /policy management/i })).toBeVisible();
    await expect(page.locator('table').first()).toBeVisible({ timeout: 15000 });
  });

  test('can draft a policy', async ({ page }) => {
    await page.goto('/policies');

    await page.getByRole('button', { name: /draft policy/i }).click();
    const container = page.getByRole('dialog').first();
    const fallbackContainer = page.locator('form').first();
    if (await container.isVisible()) {
      await expect(container).toBeVisible();
    } else {
      await expect(fallbackContainer).toBeVisible();
    }

    await page.locator('#title, input[name="title"]').first().fill('E2E Information Security Policy');
    await page.locator('#summary, textarea[name="summary"]').first().fill('E2E policy summary text.');
    await page
      .locator('#content_html, textarea[name="content_html"]')
      .first()
      .fill('<h2>Purpose</h2><p>Test policy content.</p>');

    const reviewFrequency = page.getByLabel(/review frequency/i);
    if (await reviewFrequency.isVisible()) {
      await reviewFrequency.fill('12');
    }

    await page.getByRole('button', { name: /save draft|create|submit/i }).click();

    await expect(page.getByText(/policy created|draft|success/i).first()).toBeVisible({ timeout: 15000 });
  });

  test('policy detail supports publish workflow action when available', async ({ page }) => {
    await page.goto('/policies');

    const firstPolicyLink = page.locator('a[href^="/policies/"]').first();
    if (await firstPolicyLink.isVisible()) {
      await firstPolicyLink.click();
      await expect(page).toHaveURL(/\/policies\/.+/);

      const publishButton = page.getByRole('button', { name: /publish|submit for review|acknowledge/i });
      if (await publishButton.first().isVisible()) {
        await publishButton.first().click();
      }
    }
  });
});
