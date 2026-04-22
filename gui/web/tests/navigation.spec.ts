import { test, expect } from '@playwright/test'

// All 6 IA routes. Each should render without throwing in the console
// on any viewport project.
const paths = [
  { path: '/',         name: 'Now' },
  { path: '/servers',  name: 'Servers' },
  { path: '/traffic',  name: 'Traffic' },
  { path: '/mesh',     name: 'Mesh' },
  { path: '/activity', name: 'Activity' },
  { path: '/settings', name: 'Settings' },
]

for (const { path, name } of paths) {
  test(`${name} (${path}) loads without console errors`, async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (e) => errors.push(`pageerror: ${e}`))
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(`console.error: ${msg.text()}`)
    })

    await page.goto(`/#${path}`, { waitUntil: 'domcontentloaded' })
    // Wait for the AppShell to mount (it wraps every page). networkidle
    // doesn't work here because Vite proxies /api/* to a non-running Go
    // backend in test env — those connections hang indefinitely.
    await page.waitForSelector('.shell', { timeout: 5000 }).catch(() => {})
    await page.waitForTimeout(300)

    // Ignore expected errors when the backend isn't reachable in test env —
    // Vite proxies /api/* to a Go backend that isn't running in CI, so
    // expect 500 / refused / network errors for those fetches.
    const real = errors.filter((e) =>
      !e.includes('Failed to fetch') &&
      !e.includes('ERR_CONNECTION_REFUSED') &&
      !e.includes('NetworkError') &&
      !e.includes('net::ERR_') &&
      !e.includes('Failed to load resource') &&
      !e.includes('500 (Internal Server Error)') &&
      !e.includes('502') && !e.includes('503') && !e.includes('504')
    )
    expect(real, `unexpected errors on ${path}:\n${real.join('\n')}`).toEqual([])
  })
}

test.describe('legacy route migration', () => {
  // Redirects fire synchronously in the router's `update()` via
  // history.replaceState. `page.url()` may not pick that up reliably
  // (some Chromium versions lag on replaceState), so read window.location
  // directly via page.evaluate to get the authoritative post-replace hash.
  async function readHash(page: import('@playwright/test').Page, legacy: string): Promise<string> {
    await page.goto(`/#${legacy}`, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector('.shell', { timeout: 5000 }).catch(() => {})
    await page.waitForTimeout(200)
    return page.evaluate(() => window.location.hash)
  }

  test('/dashboard redirects to /', async ({ page }) => {
    const hash = await readHash(page, '/dashboard')
    expect(hash).toMatch(/^#\/($|\?)/)
  })

  test('/subscriptions redirects to /servers?source=subscriptions', async ({ page }) => {
    const hash = await readHash(page, '/subscriptions')
    expect(hash).toContain('/servers')
    expect(hash).toContain('source=subscriptions')
  })

  test('/routing redirects to /traffic', async ({ page }) => {
    const hash = await readHash(page, '/routing')
    expect(hash).toMatch(/^#\/traffic(\?|$)/)
  })

  test('/logs redirects to /activity?tab=logs', async ({ page }) => {
    const hash = await readHash(page, '/logs')
    expect(hash).toContain('/activity')
    expect(hash).toContain('tab=logs')
  })

  test('/groups redirects to /servers (no dangling view tag)', async ({ page }) => {
    const hash = await readHash(page, '/groups')
    expect(hash).toMatch(/^#\/servers($|\?)/)
    expect(hash).not.toMatch(/view=groups/)
  })
})
