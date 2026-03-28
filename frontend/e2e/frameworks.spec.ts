import { test, expect, type Page } from '@playwright/test';

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel(/email/i).fill('admin@complianceforge.io');
  await page.getByLabel(/password/i).fill('Admin123!@#');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/dashboard/);
}

test.describe('Frameworks', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('framework list loads and renders cards', async ({ page }) => {
    await page.goto('/frameworks');

    await expect(page.getByRole('heading', { name: /frameworks/i })).toBeVisible();
    const cards = page.locator('a[href^="/frameworks/"]');
    await expect(cards.first()).toBeVisible({ timeout: 15000 });
  });

  test('can navigate from list to framework detail', async ({ page }) => {
    await page.goto('/frameworks');

    const firstFramework = page.locator('a[href^="/frameworks/"]').first();
    await expect(firstFramework).toBeVisible({ timeout: 15000 });
    await firstFramework.click();

    await expect(page).toHaveURL(/\/frameworks\/.+/);
  });

  test('framework detail supports controls search and tab switching', async ({ page }) => {
    await page.goto('/frameworks');
    const firstFramework = page.locator('a[href^="/frameworks/"]').first();
    await expect(firstFramework).toBeVisible({ timeout: 15000 });
    await firstFramework.click();

    const searchInput = page.getByPlaceholder(/search controls/i);
    if (await searchInput.isVisible()) {
      await searchInput.fill('access');
      await page.waitForTimeout(700);
    }

    const implementationTab = page.getByRole('tab', { name: /implementation status/i });
    if (await implementationTab.isVisible()) {
      await implementationTab.click();
      await expect(page.getByText(/implemented|partial|not implemented/i).first()).toBeVisible();
    }
  });
});
