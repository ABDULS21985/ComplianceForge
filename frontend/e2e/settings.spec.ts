import { test, expect, type Page } from '@playwright/test';

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel(/email/i).fill('admin@complianceforge.io');
  await page.getByLabel(/password/i).fill('Admin123!@#');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/dashboard/);
}

test.describe('Settings', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('organisation tab loads details', async ({ page }) => {
    await page.goto('/settings');

    await expect(page.getByRole('tab', { name: /organisation/i })).toBeVisible();
    await expect(page.getByText(/organisation details/i)).toBeVisible({ timeout: 15000 });
  });

  test('users tab shows users list and add user dialog opens', async ({ page }) => {
    await page.goto('/settings');

    await page.getByRole('tab', { name: /users/i }).click();
    await expect(page.getByText(/user directory|users/i).first()).toBeVisible({ timeout: 15000 });

    await page.getByRole('button', { name: /add user/i }).click();
    await expect(page.getByRole('dialog')).toBeVisible();

    await page.locator('#email, input[name="email"]').first().fill('e2e.user@example.com');
    await page.locator('#first_name, input[name="first_name"]').first().fill('E2E');
    await page.locator('#last_name, input[name="last_name"]').first().fill('User');
    await page.locator('#password, input[name="password"]').first().fill('StrongPass123!');
  });

  test('audit log tab renders entries', async ({ page }) => {
    await page.goto('/settings');

    await page.getByRole('tab', { name: /audit log/i }).click();
    await expect(page.getByText(/immutable audit trail/i)).toBeVisible({ timeout: 15000 });
  });
});
