/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const api = process.env.ARC_API_URL || 'http://localhost:8000'

// The Go API serves /api, /mcp, /webhooks, OAuth and /health; in dev Vite proxies them.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: Object.fromEntries(
      ['/api', '/mcp', '/webhooks', '/health', '/oauth', '/.well-known', '/openapi.json', '/jobs'].map(p => [p, { target: api, changeOrigin: false }]),
    ),
  },
  build: { outDir: 'dist', chunkSizeWarningLimit: 1200 },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
