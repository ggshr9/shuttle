import { test, expect } from '@playwright/test';

test.describe('P8 logs', () => {
    test.beforeEach(async ({ page }) => {
        await page.goto('/#/logs');
        await expect(page.locator('.sidebar')).toBeVisible();
    });

    test('logs tab is visible in navigation', async ({ page }) => {
        const logsTab = page.locator('a.item:has-text("Logs")');
        await expect(logsTab).toBeVisible();
    });

    test('logs page renders section title and search', async ({ page }) => {
        await expect(page.locator('h3:has-text("Logs")').first()).toBeVisible({ timeout: 5000 });
        await expect(page.getByPlaceholder(/search logs/i)).toBeVisible();
    });

    test('logs page shows the three-column layout', async ({ page }) => {
        // Level filter chips live in the left rail
        await expect(page.locator('button.chip:has-text("Info")')).toBeVisible();
        // Detail panel placeholder on the right before any selection
        await expect(page.locator('text=Select a log entry')).toBeVisible();
    });
});
