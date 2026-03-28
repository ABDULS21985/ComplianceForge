import { test, expect } from '@playwright/test';

test.describe('Authentication', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
  });

  test('should login with valid credentials and redirect to dashboard', async ({ page }) => {
    await page.getByLabel('Email').fill('admin@complianceforge.io');
    await page.getByLabel('Password').fill('Admin123!@#');
    await page.getByRole('button', { name: /sign in|log in/i }).click();

    await expect(page).toHaveURL(/.*dashboard/);
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
  });

  test('should show error message with invalid credentials', async ({ page }) => {
    await page.getByLabel('Email').fill('wrong@example.com');
    await page.getByLabel('Password').fill('wrongpassword');
    await page.getByRole('button', { name: /sign in|log in/i }).click();

    await expect(page.getByText(/invalid credentials|invalid email or password|authentication failed/i)).toBeVisible();
    await expect(page).toHaveURL(/.*login/);
  });

  test('should redirect unauthenticated users to login page', async ({ page }) => {
    await page.goto('/dashboard');

    await expect(page).toHaveURL(/.*login/);
  });

  test('should redirect unauthenticated users accessing risks page to login', async ({ page }) => {
    await page.goto('/risks');

    await expect(page).toHaveURL(/.*login/);
  });

  test('should logout and redirect to login page', async ({ page }) => {
    // Login first
    await page.getByLabel('Email').fill('admin@complianceforge.io');
    await page.getByLabel('Password').fill('Admin123!@#');
    await page.getByRole('button', { name: /sign in|log in/i }).click();
    await expect(page).toHaveURL(/.*dashboard/);

    // Logout
    await page.getByRole('button', { name: /logout|sign out/i }).click();
    await expect(page).toHaveURL(/.*login/);
  });
});
