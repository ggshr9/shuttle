import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  retries: 1,
  use: {
    baseURL: 'http://localhost:5173',
    actionTimeout: 10000,
  },
  webServer: {
    command: 'npm run dev',
    port: 5173,
    reuseExistingServer: true,
  },
  projects: [
    { name: 'desktop', use: { viewport: { width: 1440, height: 900 } } },
    { name: 'tablet',  use: { viewport: { width: 820,  height: 1180 } } },
    { name: 'phone',   use: { ...devices['iPhone 14'] } },
  ],
})
