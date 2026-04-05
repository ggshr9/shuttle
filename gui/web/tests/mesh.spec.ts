import { test, expect } from '@playwright/test';

test.describe('Mesh Page', () => {
    test.beforeEach(async ({ page }) => {
        // Go to advanced mode by setting localStorage before navigation
        await page.goto('/');
        await page.evaluate(() => localStorage.setItem('shuttle_ui_mode', 'advanced'));
        await page.reload();
        // Wait for sidebar to confirm advanced mode is active
        await expect(page.locator('.sidebar')).toBeVisible();
    });

    test('mesh tab is visible in navigation', async ({ page }) => {
        // Nav items use i18n — "Mesh" is the English label for nav.mesh
        const meshTab = page.locator('.nav-item:has-text("Mesh")');
        await expect(meshTab).toBeVisible();
    });

    test('mesh page renders when clicked', async ({ page }) => {
        await page.locator('.nav-item:has-text("Mesh")').click();
        // The Mesh page has a .page container with an h2 "Mesh VPN"
        await expect(page.locator('h2:has-text("Mesh VPN")')).toBeVisible();
    });

    test('mesh page shows status card', async ({ page }) => {
        await page.locator('.nav-item:has-text("Mesh")').click();
        // Status card is always present (shows "not enabled" or status grid)
        await expect(page.locator('.status-card')).toBeVisible();
    });

    test('mesh page shows peers section header', async ({ page }) => {
        await page.locator('.nav-item:has-text("Mesh")').click();
        // "Peers" section header is always rendered
        await expect(page.locator('h3:has-text("Peers")')).toBeVisible();
    });

    test('mesh page shows topology section', async ({ page }) => {
        await page.locator('.nav-item:has-text("Mesh")').click();
        // "Network Topology" section header
        await expect(page.locator('h3:has-text("Network Topology")')).toBeVisible();
    });
});
