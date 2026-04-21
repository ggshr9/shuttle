import { test, expect } from '@playwright/test';

test.describe('P7 mesh', () => {
    test.beforeEach(async ({ page }) => {
        await page.goto('/#/mesh');
        await expect(page.locator('.sidebar')).toBeVisible();
    });

    test('mesh tab is visible in navigation', async ({ page }) => {
        const meshTab = page.locator('a.item:has-text("Mesh")');
        await expect(meshTab).toBeVisible();
    });

    test('mesh page renders section title', async ({ page }) => {
        await expect(page.locator('h3:has-text("Mesh")').first()).toBeVisible({ timeout: 5000 });
    });
});
