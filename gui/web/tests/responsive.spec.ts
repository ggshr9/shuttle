import { test, expect } from '@playwright/test'

test.describe('responsive shell', () => {
  test('phone viewport shows BottomTabs', async ({ page, viewport }) => {
    test.skip((viewport?.width ?? 9999) >= 720, 'phone project only')
    await page.goto('/')
    const tabs = page.locator('[aria-label="Primary navigation"][role="tablist"]')
    await expect(tabs).toBeVisible()
  })

  test('tablet viewport shows Rail', async ({ page, viewport }) => {
    const w = viewport?.width ?? 0
    test.skip(w < 720 || w >= 1024, 'tablet project only')
    await page.goto('/')
    await expect(page.locator('aside.rail')).toBeVisible()
  })

  test('desktop viewport shows Sidebar', async ({ page, viewport }) => {
    test.skip((viewport?.width ?? 0) < 1024, 'desktop project only')
    await page.goto('/')
    await expect(page.locator('aside.sidebar')).toBeVisible()
  })
})
