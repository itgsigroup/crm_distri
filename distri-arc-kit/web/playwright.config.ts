import { defineConfig } from '@playwright/test'

// Smoke tests against `make dev` (API :8080 + web :5173 with the seed loaded).
export default defineConfig({
  testDir: './e2e',
  snapshotPathTemplate: '{testDir}/__screenshots__/{arg}{ext}',
  use: { baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173' },
  reporter: 'list',
})
