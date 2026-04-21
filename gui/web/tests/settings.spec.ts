import { test, expect } from '@playwright/test';

test.describe('P9 settings', () => {
    test('bare /#/settings redirects to /settings/general', async ({ page }) => {
        await page.goto('/#/settings');
        await expect(page.locator('.sidebar')).toBeVisible({ timeout: 5000 });
        await expect(page).toHaveURL(/#\/settings\/general$/);
    });

    test('settings sub-nav shows all 10 entries', async ({ page }) => {
        await page.goto('/#/settings');
        await expect(page.locator('a.sub-item').first()).toBeVisible({ timeout: 5000 });
        await expect(page.locator('a.sub-item')).toHaveCount(10);
    });

    test('clicking a sub-nav entry updates the URL', async ({ page }) => {
        await page.goto('/#/settings/general');
        await expect(page.locator('.sidebar')).toBeVisible();
        await page.locator('a.sub-item:has-text("DNS")').click();
        await expect(page).toHaveURL(/#\/settings\/dns$/);
    });

    test('main sidebar keeps Settings highlighted under sub-routes', async ({ page }) => {
        await page.goto('/#/settings/mesh');
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('.sidebar a.item.on')).toContainText(/settings/i);
    });

    test('Logging sub-page renders level selector', async ({ page }) => {
        await page.goto('/#/settings/logging');
        await expect(page.locator('.sidebar')).toBeVisible({ timeout: 5000 });
        // Field label for log level should be present.
        await expect(page.locator('text=/log\\s*level/i').first()).toBeVisible();
    });

    test('unsaved-bar stays hidden until config loads cleanly', async ({ page }) => {
        await page.goto('/#/settings/general');
        await expect(page.locator('.sidebar')).toBeVisible();
        // Without backend mutations the draft equals pristine → bar absent.
        await expect(page.locator('text=/unsaved changes/i')).not.toBeVisible();
    });
});
