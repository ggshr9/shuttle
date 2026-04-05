import { test, expect } from '@playwright/test';

test.describe('Subscriptions', () => {
    test.beforeEach(async ({ page }) => {
        await page.goto('/');
        await page.evaluate(() => localStorage.setItem('shuttle_ui_mode', 'advanced'));
        await page.reload();
        await expect(page.locator('.sidebar')).toBeVisible();
    });

    test('subscriptions tab is accessible', async ({ page }) => {
        const subTab = page.locator('.nav-item:has-text("Subscriptions")');
        await expect(subTab).toBeVisible();
    });

    test('subscriptions page renders when clicked', async ({ page }) => {
        await page.locator('.nav-item:has-text("Subscriptions")').click();
        // The Subscriptions page has an h2 with "Subscriptions" title
        await expect(page.locator('h2:has-text("Subscriptions")')).toBeVisible();
    });

    test('subscriptions page shows add button', async ({ page }) => {
        await page.locator('.nav-item:has-text("Subscriptions")').click();
        // "Add Subscription" button in the page header
        const addBtn = page.locator('.btn-primary:has-text("Add Subscription")');
        await expect(addBtn).toBeVisible();
    });

    test('add subscription dialog opens', async ({ page }) => {
        await page.locator('.nav-item:has-text("Subscriptions")').click();
        await page.locator('.btn-primary:has-text("Add Subscription")').click();
        // Modal should appear with the dialog
        const modal = page.locator('.modal');
        await expect(modal).toBeVisible();
        // Should have Name and URL inputs
        await expect(modal.locator('input').first()).toBeVisible();
    });

    test('add subscription dialog can be closed', async ({ page }) => {
        await page.locator('.nav-item:has-text("Subscriptions")').click();
        await page.locator('.btn-primary:has-text("Add Subscription")').click();
        await expect(page.locator('.modal')).toBeVisible();
        // Close via the X button
        await page.locator('.modal-close').click();
        await expect(page.locator('.modal')).not.toBeVisible();
    });

    test('add button is disabled without URL', async ({ page }) => {
        await page.locator('.nav-item:has-text("Subscriptions")').click();
        await page.locator('.btn-primary:has-text("Add Subscription")').click();
        // The "Add" submit button inside the modal should be disabled when URL is empty
        const submitBtn = page.locator('.modal-footer .btn-primary');
        await expect(submitBtn).toBeDisabled();
    });
});
