import { test, expect } from '@playwright/test'

test.describe('responsive shell', () => {
  test('phone viewport shows BottomTabs', async ({ page, viewport }) => {
    test.skip((viewport?.width ?? 9999) >= 720, 'phone project only')
    await page.goto('/')
    const tabs = page.locator('[aria-label="Primary navigation"][role="tablist"]')
    await expect(tabs).toBeVisible()
  })

  test('desktop viewport shows Sidebar', async ({ page, viewport }) => {
    test.skip((viewport?.width ?? 0) < 1024, 'desktop project only')
    await page.goto('/')
    await expect(page.locator('aside').first()).toBeVisible()
  })
})
