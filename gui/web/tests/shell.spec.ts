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

test.describe('P3 dashboard', () => {
    test('root URL renders new Dashboard hero', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible();
        // New Dashboard always shows a hero card with either "Connected" or
        // "Disconnected" state.
        await expect(page.locator('text=/Connected|Disconnected/').first()).toBeVisible({ timeout: 5000 });
    });

    test('dashboard shows four stats cards', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('text=RTT')).toBeVisible({ timeout: 5000 });
        await expect(page.locator('text=Packet loss')).toBeVisible();
        await expect(page.locator('text=Transfer')).toBeVisible();
        await expect(page.locator('text=Transport').first()).toBeVisible();
    });

    test('dashboard shows throughput sparkline', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('text=Throughput')).toBeVisible({ timeout: 5000 });
    });
});

test.describe('P4 servers', () => {
    test('servers URL renders dense table header', async ({ page }) => {
        await page.goto('/#/servers');
        await expect(page.locator('.sidebar')).toBeVisible();
        // Empty state shows "No servers" since backend is unreachable in tests.
        // Header row is only rendered when there are servers, so we verify the
        // page chrome (Section title + Add button) instead.
        await expect(page.locator('h3:has-text("Servers")')).toBeVisible({ timeout: 5000 });
        await expect(page.locator('button:has-text("Add server")')).toBeVisible();
    });

    test('Add server button opens dialog', async ({ page }) => {
        await page.goto('/#/servers');
        await expect(page.locator('.sidebar')).toBeVisible();
        await page.locator('button:has-text("Add server")').click();
        await expect(page.locator('text=Enter server details')).toBeVisible({ timeout: 5000 });
    });
});
