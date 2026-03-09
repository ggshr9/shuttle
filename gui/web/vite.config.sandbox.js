// Vite config for sandbox/Docker GUI testing.
// Usage: npm run dev:sandbox
//
// Connects the Svelte frontend to client-a running in Docker sandbox.
// Client A API: http://localhost:19091
// Client B API: http://localhost:19092

import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

const SANDBOX_CLIENT = process.env.SANDBOX_CLIENT || 'a'
const API_PORT = SANDBOX_CLIENT === 'b' ? 19092 : 19091

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: `http://localhost:${API_PORT}`,
        // WebSocket support for live log/connection streams
        ws: true,
      },
    },
  },
})
