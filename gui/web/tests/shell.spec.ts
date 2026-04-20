import { test, expect } from '@playwright/test';

test.describe('P2 shell', () => {
    test('root URL renders shell with sidebar', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible({ timeout: 5000 });
    });

    test('sidebar has three section headings', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible();
        // Overview / Network / System
        const headings = page.locator('.sidebar .heading');
        await expect(headings).toHaveCount(3);
    });

    test('navigating to /#/servers highlights Servers in sidebar', async ({ page }) => {
        await page.goto('/#/servers');
        await expect(page.locator('.sidebar')).toBeVisible();
        const active = page.locator('a.item.on');
        await expect(active).toBeVisible();
        await expect(active).toContainText(/server/i);
    });

    test('clicking a sidebar link updates the URL hash', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible();
        await page.locator('a.item:has-text("Logs")').click();
        await expect(page).toHaveURL(/#\/logs$/);
    });
});
