import { test, expect } from '@playwright/test';

// The onboarding wizard only appears when the backend reports zero
// configured servers. The test harness mounts against an unreachable
// API (apiError path) so we drive the wizard directly via a simple
// flag trigger: App.svelte treats a rejected getConfig() as "no
// onboarding". For deterministic coverage we stub /api/config to
// return an empty config and verify the 4-step flow + DotProgress.

test.describe('P10 onboarding', () => {
    test.beforeEach(async ({ page }) => {
        await page.route('**/api/config', (route) => {
            route.fulfill({
                status: 200,
                contentType: 'application/json',
                body: JSON.stringify({ proxy: {} }),
            });
        });
    });

    test('wizard mounts on a fresh install', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('[role="dialog"]')).toBeVisible({ timeout: 5000 });
        await expect(page.locator('h1:has-text("Welcome to Shuttle")')).toBeVisible();
    });

    test('dot progress has exactly four dots with one on', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('[role="dialog"]')).toBeVisible({ timeout: 5000 });
        const dots = page.locator('.dots .dot');
        await expect(dots).toHaveCount(4);
        await expect(page.locator('.dots .dot.on')).toHaveCount(1);
    });

    test('Next advances step and adds a second lit dot', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('h1:has-text("Welcome to Shuttle")')).toBeVisible({ timeout: 5000 });
        await page.locator('button:has-text("Next")').click();
        await expect(page.locator('h1:has-text("Add a Server")')).toBeVisible();
        await expect(page.locator('.dots .dot.on')).toHaveCount(2);
    });

    test('Back on step 2 returns to step 1', async ({ page }) => {
        await page.goto('/');
        await page.locator('button:has-text("Next")').click();
        await expect(page.locator('h1:has-text("Add a Server")')).toBeVisible({ timeout: 5000 });
        await page.locator('button:has-text("Back")').click();
        await expect(page.locator('h1:has-text("Welcome to Shuttle")')).toBeVisible();
    });

    test('method tabs switch the form body', async ({ page }) => {
        await page.goto('/');
        await page.locator('button:has-text("Next")').click();
        await expect(page.locator('h1:has-text("Add a Server")')).toBeVisible({ timeout: 5000 });
        await page.locator('button.tab:has-text("Manual")').click();
        await expect(page.getByPlaceholder(/server\.example\.com/i)).toBeVisible();
    });
});
