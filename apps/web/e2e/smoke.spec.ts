import { expect, test } from '@playwright/test'
import { login, trackErrors } from './helpers'

const SCREENS: [string, RegExp][] = [
  ['today', /Selamat (pagi|siang|sore|malam)/],
  ['chat', /Chat/],
  ['rel', /Relasi|RSUD/],
  ['net', /Peta/],
  ['pipe', /Forecast|Penjualan/],
  ['pros', /Prospek|Funnel/],
  ['cash', /Kas|piutang/i],
  ['ask', /Tanya|ARC/],
  ['conn', /Pengaturan|Sumber/],
]

test('every screen renders from the API without page errors', async ({ page }) => {
  const errors = trackErrors(page)
  await login(page)
  for (const [screen, text] of SCREENS) {
    await page.goto('/#' + screen)
    await expect(page.locator('main.stage')).toContainText(text, { timeout: 10_000 })
  }
  expect(errors).toEqual([])
})

test('400 px wide: no screen scrolls horizontally', async ({ page }) => {
  await page.setViewportSize({ width: 400, height: 860 })
  await login(page)
  for (const [screen] of SCREENS) {
    await page.goto('/#' + screen)
    await page.waitForTimeout(800)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow, screen).toBeLessThanOrEqual(0)
  }
})

test('forecast and cash numbers match the approved mockup', async ({ page }) => {
  await login(page)
  await page.goto('/#today')
  await expect(page.getByText('Rp 4,3 M').first()).toBeVisible()
  await expect(page.getByText('Rp 5,9 M').first()).toBeVisible()
  await page.goto('/#cash')
  await expect(page.getByText('Rp 2,4 M').first()).toBeVisible()
})

test('sales Semarang cannot open a Yogyakarta account', async ({ page }) => {
  await login(page, 'andi@gsi.co.id')
  const res = await page.request.get('/api/accounts/sleman')
  expect([403, 404]).toContain(res.status())
})

test('action sheet opens with provenance and closes without deciding', async ({ page }) => {
  await login(page)
  await page.goto('/#today')
  await page.locator('.q.open .q-body button:has-text("Edit")').first().click()
  const sheet = page.locator('.sheet.show')
  await expect(sheet).toBeVisible()
  await expect(sheet.locator('#sheet-title')).not.toBeEmpty()
  await page.keyboard.press('Escape')
  await expect(page.locator('.sheet.show')).toHaveCount(0)
})
