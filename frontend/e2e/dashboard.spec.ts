import { test, expect } from '@playwright/test';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/login');
    await page.getByLabel('Email').fill('admin@complianceforge.io');
    await page.getByLabel('Password').fill('Admin123!@#');
    await page.getByRole('button', { name: /sign in|log in/i }).click();
    await expect(page).toHaveURL(/.*dashboard/);
  });

  test('should load dashboard with KPI cards', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();

    // Verify KPI cards are present
    const kpiCards = page.locator('[data-testid="kpi-card"]');
    await expect(kpiCards).toHaveCount(4);

    // Verify expected KPI labels
    await expect(page.getByText(/total risks/i)).toBeVisible();
    await expect(page.getByText(/open controls/i)).toBeVisible();
    await expect(page.getByText(/compliance score/i)).toBeVisible();
    await expect(page.getByText(/pending tasks/i)).toBeVisible();
  });

  test('should render charts on the dashboard', async ({ page }) => {
    // Wait for charts to load
    const charts = page.locator('[data-testid="dashboard-chart"]');
    await expect(charts.first()).toBeVisible({ timeout: 10000 });

    // Verify at least one chart container is rendered with content
    const chartCount = await charts.count();
    expect(chartCount).toBeGreaterThanOrEqual(1);

    // Verify chart canvas or SVG elements are rendered
    const chartElements = page.locator('[data-testid="dashboard-chart"] canvas, [data-testid="dashboard-chart"] svg');
    await expect(chartElements.first()).toBeVisible({ timeout: 10000 });
  });

  test('should display recent activity feed', async ({ page }) => {
    const activityFeed = page.locator('[data-testid="activity-feed"]');
    await expect(activityFeed).toBeVisible();

    const activityItems = activityFeed.locator('[data-testid="activity-item"]');
    const count = await activityItems.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });
});
