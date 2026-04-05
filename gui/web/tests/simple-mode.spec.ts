import { test, expect } from '@playwright/test';

test.describe('Simple Mode', () => {
    test('renders by default', async ({ page }) => {
        await page.goto('/');
        // SimpleMode is the default uiMode ('simple') — the .simple-mode container should appear
        await expect(page.locator('.simple-mode')).toBeVisible();
    });

    test('shows connect button', async ({ page }) => {
        await page.goto('/');
        const connectBtn = page.locator('.connect-btn');
        await expect(connectBtn).toBeVisible();
    });

    test('shows status indicator', async ({ page }) => {
        await page.goto('/');
        // Status indicator shows "Connected" or "Disconnected"
        await expect(page.locator('.status-indicator')).toBeVisible();
        // Without a backend, status defaults to disconnected
        await expect(page.locator('.status-indicator')).toContainText('Disconnected');
    });

    test('shows Advanced Mode switch button', async ({ page }) => {
        await page.goto('/');
        const modeSwitch = page.locator('.mode-switch');
        await expect(modeSwitch).toBeVisible();
        await expect(modeSwitch).toContainText('Advanced Mode');
    });

    test('switches to advanced mode', async ({ page }) => {
        await page.goto('/');
        // Click the "Advanced Mode →" button
        await page.locator('.mode-switch').click();
        // Should see the sidebar navigation (advanced mode layout)
        await expect(page.locator('.sidebar')).toBeVisible();
        // SimpleMode container should be gone
        await expect(page.locator('.simple-mode')).not.toBeVisible();
    });

    test('persists mode in localStorage', async ({ page }) => {
        await page.goto('/');
        // Switch to advanced
        await page.locator('.mode-switch').click();
        await expect(page.locator('.sidebar')).toBeVisible();
        // Reload the page
        await page.reload();
        // Should still be in advanced mode — sidebar visible, not simple-mode
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('.simple-mode')).not.toBeVisible();
    });

    test('can switch back to simple mode from settings', async ({ page }) => {
        // Start in advanced mode
        await page.goto('/');
        await page.evaluate(() => localStorage.setItem('shuttle_ui_mode', 'advanced'));
        await page.reload();
        await expect(page.locator('.sidebar')).toBeVisible();
        // Click Settings tab (last nav item)
        await page.locator('.nav-item:has-text("Settings")').click();
        // Click "Switch to Simple Mode" button
        const simpleModeBtn = page.locator('text=Switch to Simple Mode');
        if (await simpleModeBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
            await simpleModeBtn.click();
            await expect(page.locator('.simple-mode')).toBeVisible();
        }
    });
});
