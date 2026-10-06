/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The API runs on :8080 (arc api). For convenience the dev proxy authenticates as ARC_DEV_USER through the X-Dev-User
// header (accepted only when APP_ENV=dev); a session cookie from /login wins. ARC_DEV_USER= (empty) → login required.
const api = process.env.ARC_API_URL ?? 'http://localhost:8080'
const devUser = process.env.ARC_DEV_USER ?? 'sam@gsi.co.id'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // the guide test compares src/features/guide/panduan.html with the approved mockup in ../reference
    fs: { allow: ['.', '../reference'] },
    proxy: {
      '/api': { target: api, headers: devUser ? { 'X-Dev-User': devUser } : undefined },
      '/mcp': { target: api },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
