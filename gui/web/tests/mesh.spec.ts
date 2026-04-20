import { test, expect } from '@playwright/test';

test.describe('Mesh Page', () => {
    test.beforeEach(async ({ page }) => {
        // New router: navigate directly via hash URL.
        await page.goto('/#/mesh');
        // Shell renders the sidebar; wait for it before asserting content.
        await expect(page.locator('.sidebar')).toBeVisible();
    });

    test('mesh tab is visible in navigation', async ({ page }) => {
        const meshTab = page.locator('a.item:has-text("Mesh")');
        await expect(meshTab).toBeVisible();
    });

    test('mesh page renders', async ({ page }) => {
        await expect(page.locator('h2:has-text("Mesh VPN")')).toBeVisible();
    });

    test('mesh page shows status card', async ({ page }) => {
        await expect(page.locator('.status-card')).toBeVisible();
    });

    test('mesh page shows peers section header', async ({ page }) => {
        await expect(page.locator('h3:has-text("Peers")')).toBeVisible();
    });

    test('mesh page shows topology section', async ({ page }) => {
        await expect(page.locator('h3:has-text("Network Topology")')).toBeVisible();
    });
});
