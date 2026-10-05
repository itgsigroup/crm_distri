/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The API runs on :8080 (arc api). Until sessions exist (stage 11) the dev proxy authenticates as the CEO
// through the X-Dev-User header, which the API accepts only when APP_ENV=dev.
const api = process.env.ARC_API_URL ?? 'http://localhost:8080'
const devUser = process.env.ARC_DEV_USER ?? 'sam@gsi.co.id'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: api, headers: { 'X-Dev-User': devUser } },
      '/mcp': { target: api },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
