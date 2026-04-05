import { defineConfig } from '@playwright/test';

export default defineConfig({
    testDir: './tests',
    timeout: 30000,
    retries: 1,
    use: {
        baseURL: 'http://localhost:5173',
        // No backend available in test — API calls will fail gracefully
        actionTimeout: 10000,
    },
    webServer: {
        command: 'npm run dev',
        port: 5173,
        reuseExistingServer: true,
    },
});
