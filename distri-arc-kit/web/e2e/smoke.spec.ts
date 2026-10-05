import { expect, test } from '@playwright/test'

const routes = [
  ['pusat-kendali', '/', 'Rencana hari ini'],
  ['orbit', '/orbit', 'Isi orbit'],
  ['segmen', '/orbit/segmen', 'Isi segmen'],
  ['dealer-mitra', '/dealer/mitra', 'CV Mitra Jaya Teknik'],
  ['orchestrator', '/orchestrator', 'Orchestrator'],
  ['chat', '/chat', 'Grup internal'],
] as const

for (const [name, path, text] of routes) {
  for (const width of [1440, 390]) {
    test(`${name} @${width}`, async ({ page }) => {
      const errors: string[] = []
      page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
      page.on('pageerror', (e) => errors.push(String(e)))
      await page.setViewportSize({ width, height: 900 })
      await page.goto(path, { waitUntil: 'networkidle' })
      await expect(page.locator('main').getByText(text).first()).toBeVisible()
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
      expect(overflow, 'no horizontal scroll').toBeLessThanOrEqual(0)
      await page.screenshot({ path: `e2e/__screenshots__/${name}-${width}.png`, fullPage: true })
      expect(errors).toEqual([])
    })
  }
}

test('Pusat kendali shows the numbers from the API', async ({ page }) => {
  await page.goto('/', { waitUntil: 'networkidle' })
  const strip = page.locator('.status-strip')
  await expect(strip.getByText('Jadwal order')).toBeVisible()
  await expect(strip.locator('button').nth(1).locator('.n')).toHaveText('6')
  await expect(strip.locator('button').nth(2).locator('.n')).toHaveText('4')
  await expect(page.locator('.dn')).toHaveCount(0)
  await page.goto('/orbit', { waitUntil: 'networkidle' })
  await expect(page.locator('svg.orbit .dn')).toHaveCount(18)
  await page.goto('/dealer/mitra', { waitUntil: 'networkidle' })
  await expect(page.locator('.acc-head .ring text')).toHaveText('44')
})

test('a proposal opens in the ActionSheet with its provenance (read-only)', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.goto('/', { waitUntil: 'networkidle' })
  const btn = page.locator('.row-list > li .btn.primary').first()
  test.skip(!(await btn.count()), 'no open proposals (run bin/arc ctl agents run --all)')
  await btn.click()
  await expect(page.getByText('Kenapa sekarang')).toBeVisible()
  await expect(page.getByRole('button', { name: /Setujui & jalankan/ })).toBeVisible()
  await page.locator('.ft').getByRole('button', { name: 'Nanti' }).click()
  expect(errors).toEqual([])
})

test('Analisis ulang di Orbit runs a cycle and reports it (stage 06)', async ({ page }) => {
  await page.goto('/orbit', { waitUntil: 'networkidle' })
  const before = await page.locator('.orch-pill .op-t b').innerText()
  await page.getByRole('button', { name: 'Analisis ulang' }).first().click()
  await expect(page.locator('.dock')).toHaveClass(/running/)
  await expect(page.locator('.toast')).toContainText('Analisis ulang orbit selesai', { timeout: 10_000 })
  await expect(page.locator('.orch-pill .op-t b')).not.toHaveText(before)
  await page.goto('/orchestrator', { waitUntil: 'networkidle' })
  await expect(page.locator('table.runs tbody tr').first()).toContainText('Analisis ulang orbit')
  await expect(page.locator('.conflicts li')).not.toHaveCount(0)
})
