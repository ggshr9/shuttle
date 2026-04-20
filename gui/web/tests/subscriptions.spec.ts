import { test, expect } from '@playwright/test';

test.describe('P5 subscriptions', () => {
    test('subscriptions URL renders page chrome', async ({ page }) => {
        await page.goto('/#/subscriptions');
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('h3:has-text("Subscriptions")')).toBeVisible({ timeout: 5000 });
        await expect(page.locator('button:has-text("Add subscription")')).toBeVisible();
    });

    test('Add subscription button opens dialog', async ({ page }) => {
        await page.goto('/#/subscriptions');
        await expect(page.locator('.sidebar')).toBeVisible();
        await page.locator('button:has-text("Add subscription")').click();
        await expect(page.locator('text=Paste a subscription URL')).toBeVisible({ timeout: 5000 });
    });
});
