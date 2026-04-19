import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.js'

export default mergeConfig(
  viteConfig,
  defineConfig({
    resolve: {
      // Force Svelte's client (browser) build when running under jsdom;
      // without this Svelte 5 loads its SSR entry and throws
      // lifecycle_function_unavailable on mount.
      conditions: ['browser'],
    },
    test: {
      environment: 'jsdom',
      globals: true,
      setupFiles: ['./test/setup.ts'],
      include: ['src/**/*.{test,spec}.{ts,svelte.ts}'],
    },
  }),
)
