import { createHmac } from 'node:crypto'
import { expect, type APIRequestContext, type Page } from '@playwright/test'

export const API = process.env.ARC_API_URL ?? 'http://localhost:8000'
const BRIDGE_SECRET = process.env.BRIDGE_SECRET ?? 'dev-bridge-secret'

export async function login(page: Page, email = 'sam@gsi.co.id') {
  await page.goto('/')
  await page.fill('input[type=email]', email)
  await page.fill('input[type=password]', 'arc12345')
  await page.click('button:has-text("Masuk")')
  await expect(page.locator('main.stage')).toBeVisible()
}

/** Collects uncaught page errors so each test can assert a clean console. */
export function trackErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('pageerror', e => errors.push(e.message))
  return errors
}

/** Posts a WaEvent to the API exactly like wa-bridge does (HMAC-SHA256 of the body). */
export async function postWaEvent(request: APIRequestContext, ev: Record<string, unknown>) {
  const body = JSON.stringify({ event: { transport: 'bridge', is_group: false, from_me: false, is_history: false, timestamp: new Date().toISOString(), ...ev } })
  const sig = createHmac('sha256', BRIDGE_SECRET).update(body).digest('hex')
  const res = await request.post(API + '/webhooks/wa', { data: body, headers: { 'Content-Type': 'application/json', 'X-ARC-Signature': sig } })
  expect(res.ok(), await res.text()).toBeTruthy()
  return res.json()
}
