import { test, expect } from '@playwright/test';

test.describe('Subscriptions', () => {
    test.beforeEach(async ({ page }) => {
        await page.goto('/#/subscriptions');
        await expect(page.locator('.sidebar')).toBeVisible();
    });

    test('subscriptions tab is accessible', async ({ page }) => {
        const subTab = page.locator('a.item:has-text("Subscriptions")');
        await expect(subTab).toBeVisible();
    });

    test('subscriptions page renders', async ({ page }) => {
        await expect(page.locator('h2:has-text("Subscriptions")')).toBeVisible();
    });

    test('subscriptions page shows add button', async ({ page }) => {
        const addBtn = page.locator('.btn-primary:has-text("Add Subscription")');
        await expect(addBtn).toBeVisible();
    });

    test('add subscription dialog opens', async ({ page }) => {
        await page.locator('.btn-primary:has-text("Add Subscription")').click();
        const modal = page.locator('.modal');
        await expect(modal).toBeVisible();
        await expect(modal.locator('input').first()).toBeVisible();
    });

    test('add subscription dialog can be closed', async ({ page }) => {
        await page.locator('.btn-primary:has-text("Add Subscription")').click();
        await expect(page.locator('.modal')).toBeVisible();
        await page.locator('.modal-close').click();
        await expect(page.locator('.modal')).not.toBeVisible();
    });

    test('add button is disabled without URL', async ({ page }) => {
        await page.locator('.btn-primary:has-text("Add Subscription")').click();
        const submitBtn = page.locator('.modal-footer .btn-primary');
        await expect(submitBtn).toBeDisabled();
    });
});
