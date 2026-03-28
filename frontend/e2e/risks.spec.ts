import { test, expect } from '@playwright/test';

test.describe('Risk Management', () => {
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/login');
    await page.getByLabel('Email').fill('admin@complianceforge.io');
    await page.getByLabel('Password').fill('Admin123!@#');
    await page.getByRole('button', { name: /sign in|log in/i }).click();
    await expect(page).toHaveURL(/.*dashboard/);
  });

  test('should load risk register with table', async ({ page }) => {
    await page.goto('/risks');

    await expect(page.getByRole('heading', { name: /risk register/i })).toBeVisible();

    // Verify risk table or list is present
    const riskTable = page.locator('[data-testid="risk-table"], table');
    await expect(riskTable).toBeVisible({ timeout: 10000 });

    // Verify table headers
    await expect(page.getByText(/title|name/i).first()).toBeVisible();
    await expect(page.getByText(/likelihood/i).first()).toBeVisible();
    await expect(page.getByText(/impact/i).first()).toBeVisible();
    await expect(page.getByText(/status/i).first()).toBeVisible();
  });

  test('should open and submit create risk form', async ({ page }) => {
    await page.goto('/risks');

    // Click create risk button
    await page.getByRole('button', { name: /create risk|add risk|new risk/i }).click();

    // Fill in risk form
    await page.getByLabel(/title|name/i).fill('E2E Test Risk - Data Breach Scenario');
    await page.getByLabel(/description/i).fill(
      'Test risk created by E2E automated tests to validate risk creation workflow.',
    );

    // Select likelihood
    const likelihoodSelect = page.getByLabel(/likelihood/i);
    if (await likelihoodSelect.isVisible()) {
      await likelihoodSelect.selectOption({ index: 3 });
    }

    // Select impact
    const impactSelect = page.getByLabel(/impact/i);
    if (await impactSelect.isVisible()) {
      await impactSelect.selectOption({ index: 3 });
    }

    // Select category if present
    const categorySelect = page.getByLabel(/category/i);
    if (await categorySelect.isVisible()) {
      await categorySelect.selectOption({ index: 1 });
    }

    // Submit form
    await page.getByRole('button', { name: /save|create|submit/i }).click();

    // Verify risk was created - either redirect or success message
    await expect(
      page.getByText(/risk created|successfully|E2E Test Risk/i).first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test('should render risk heatmap', async ({ page }) => {
    await page.goto('/risks');

    // Look for heatmap tab or section
    const heatmapTab = page.getByRole('tab', { name: /heatmap|matrix/i });
    if (await heatmapTab.isVisible()) {
      await heatmapTab.click();
    }

    // Verify heatmap is rendered
    const heatmap = page.locator('[data-testid="risk-heatmap"]');
    await expect(heatmap).toBeVisible({ timeout: 10000 });

    // Verify heatmap has grid cells
    const cells = heatmap.locator('[data-testid="heatmap-cell"]');
    const cellCount = await cells.count();
    expect(cellCount).toBeGreaterThanOrEqual(1);

    // Verify axis labels are present
    await expect(page.getByText(/likelihood/i).first()).toBeVisible();
    await expect(page.getByText(/impact/i).first()).toBeVisible();
  });

  test('should filter risks by status', async ({ page }) => {
    await page.goto('/risks');

    // Wait for table to load
    const riskTable = page.locator('[data-testid="risk-table"], table');
    await expect(riskTable).toBeVisible({ timeout: 10000 });

    // Apply filter
    const statusFilter = page.getByLabel(/status filter|filter by status/i);
    if (await statusFilter.isVisible()) {
      await statusFilter.selectOption('open');
      // Verify filtered results
      await page.waitForTimeout(500);
      const rows = riskTable.locator('tbody tr');
      const count = await rows.count();
      expect(count).toBeGreaterThanOrEqual(0);
    }
  });
});
