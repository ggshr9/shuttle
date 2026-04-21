import { defineConfig, devices } from '@playwright/test'

// Mobile-unification Phase 2: these specs target pages / chrome that Phase 2
// removed or redirected. Re-enable each when the corresponding Phase 3 page
// lands and rewrite the assertions against the new IA:
//   logs.spec.ts          ← Phase 3d (Activity absorbs Logs)
//   subscriptions.spec.ts ← Phase 3b (Servers absorbs Subscriptions)
//   shell.spec.ts         ← Phase 3a+ (legacy Dashboard / Groups / Routing DOM)
const LEGACY_SPECS_PENDING_PHASE_3 = [
  '**/logs.spec.ts',
  '**/subscriptions.spec.ts',
  '**/shell.spec.ts',
]

export default defineConfig({
  testDir: './tests',
  testIgnore: LEGACY_SPECS_PENDING_PHASE_3,
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
