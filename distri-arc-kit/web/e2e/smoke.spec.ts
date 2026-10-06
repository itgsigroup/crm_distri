import { expect, test } from '@playwright/test'

const routes = [
  ['pusat-kendali', '/', 'Rencana hari ini'],
  ['orbit', '/orbit', 'Isi orbit'],
  ['segmen', '/orbit/segmen', 'Isi segmen'],
  ['dealer-mitra', '/dealer/mitra', 'CV Mitra Jaya Teknik'],
  ['orchestrator', '/orchestrator', 'Orchestrator'],
  ['chat', '/chat', 'Grup internal'],
  ['stok', '/stok', 'Stok kritis'],
  ['kredit', '/kredit', 'Prediksi kas masuk 30 hari'],
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

test('MCP panel and Koneksi AI come from the server (stage 07)', async ({ page }) => {
  await page.goto('/orchestrator', { waitUntil: 'networkidle' })
  const panel = page.locator('.card').filter({ hasText: 'MCP sebagai orchestrator' })
  await expect(panel.getByText('Terkunci')).toBeVisible()
  await expect(panel.locator('.tools')).toContainText('orchestrator.submit')
  await page.goto('/pengaturan', { waitUntil: 'networkidle' })
  await expect(page.locator('.ep')).toContainText('/mcp')
})

test('Peta relasi loads the graph from the API (stage 08)', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.goto('/orbit/relasi', { waitUntil: 'networkidle' })
  await expect(page.locator('.pairs li').nth(1)).toContainText('Rizky ↔ Indo Vision Security')
  await expect(page.locator('.pairs li').nth(1).locator('em')).toHaveText('104')
  await page.getByRole('button', { name: '180 hr' }).click()
  await expect(page.locator('.pairs li').nth(1).locator('em')).toHaveText('553')
  await expect(page.locator('.ins')).toContainText('Mitra Jaya Teknik: 30 hari tanpa order (siklus 21) — intensitas WA turun')
  await expect(page.locator('.net-lbl')).toHaveCount(22)
  expect(errors).toEqual([])
})

test('⌘K answers with sources and the memo shows sources per sentence (stage 10)', async ({ page }) => {
  await page.goto('/', { waitUntil: 'networkidle' })
  const input = page.locator('.searchwrap input').first()
  await input.fill('dealer mana yang berisiko')
  await input.press('Enter')
  await expect(page.locator('.sheet')).toContainText('Tiga dealer')
  await expect(page.locator('.sheet .prov').first()).toBeVisible()
  await page.goto('/dealer/mitra', { waitUntil: 'networkidle' })
  const s = page.locator('#sec-memo .ms').first()
  await s.click()
  await expect(page.locator('.sheet')).toContainText('Sumber klaim')
})

const demo = process.env.ARC_DEMO_PASSWORD

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.locator('input[name=email]').fill(email)
  await page.locator('input[name=password]').fill(demo!)
  await page.getByRole('button', { name: 'Masuk' }).click()
  await page.waitForURL('**/')
  await page.waitForLoadState('networkidle')
}

test('three roles see their own menu (stage 11)', async ({ page }) => {
  test.skip(!demo, 'ARC_DEMO_PASSWORD not set')
  const rail = page.locator('aside.rail')
  await login(page, 'andi@gsi.co.id')
  await expect(rail.getByRole('button', { name: 'Pengaturan' })).toHaveCount(0)
  await expect(rail.getByRole('button', { name: /Kredit/ })).toHaveCount(0)
  await expect(rail.getByRole('button', { name: 'Push stok' })).toBeVisible()
  await expect(rail.locator('.me')).toContainText('Andi')
  await login(page, 'finance@gsi.co.id')
  await expect(rail.getByRole('button', { name: /Kredit/ })).toBeVisible()
  await expect(rail.getByRole('button', { name: 'Chat' })).toHaveCount(0)
  await expect(rail.getByRole('button', { name: 'Pengaturan' })).toHaveCount(0)
  await login(page, 'sam@gsi.co.id')
  await expect(rail.getByRole('button', { name: 'Pengaturan' })).toBeVisible()
  await page.goto('/pengaturan', { waitUntil: 'networkidle' })
  await expect(page.getByText('Pengguna & peran')).toBeVisible()
  await rail.locator('.me').click()
  await page.getByRole('button', { name: 'Keluar' }).click()
  await page.waitForURL('**/login')
})

test('drift threshold 1,5× changes At risk on the next cycle; Cara baca opens Panduan (stage 11)', async ({ page, request }) => {
  const atRisk = async () => {
    const r = await request.get('/api/orbit')
    const items = (await r.json()).items as { metrics: { status: string } }[]
    return items.filter((d) => d.metrics.status === 'At risk').length
  }
  const before = await atRisk()
  await page.goto('/pengaturan', { waitUntil: 'networkidle' })
  const row = page.locator('.rules li').filter({ hasText: 'Ambang lewat jadwal' })
  try {
    await row.getByRole('button', { name: '1,5×' }).click()
    await expect(page.locator('.toast')).toContainText('berlaku di siklus berikutnya')
    await page.goto('/orbit', { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: 'Analisis ulang' }).first().click()
    await expect(page.locator('.toast')).toContainText('selesai', { timeout: 10_000 })
    expect(await atRisk()).toBeLessThan(before)
  } finally {
    await request.put('/api/policies/orbit.thresholds', { data: { drift: 1.2, churn: 2, key_account: { sow_min: 50, on_time_min: 85 } } })
    await request.post('/api/cycles', { data: { scope: 'screen:orbit' } })
  }
  await page.goto('/orbit', { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: 'Cara baca' }).click()
  await page.waitForURL('**/panduan')
  await expect(page.getByText('Satu gambar, semua istilah')).toBeVisible()
})

test('Status sistem shows each part from /api/health; 2FA setup shows a key (stage 13)', async ({ page }) => {
  await page.goto('/pengaturan', { waitUntil: 'networkidle' })
  const status = page.locator('.card').filter({ has: page.getByRole('heading', { name: 'Status sistem' }) })
  for (const part of ['Database', 'Antrean job', 'WhatsApp', 'Odoo', 'Siklus Orchestrator', 'Outbox']) {
    await expect(status.locator('.rules li').filter({ hasText: part }).first()).toBeVisible()
  }
  await expect(status).toContainText('partisi bulanan')
  const security = page.locator('.card').filter({ has: page.getByRole('heading', { name: 'Keamanan akun' }) })
  await security.getByRole('button', { name: 'Verifikasi dua langkah' }).click()
  await page.getByRole('button', { name: 'Buat kunci' }).click()
  await expect(page.getByText('Kunci setup')).toBeVisible()
  await expect(page.getByLabel('Kode 2FA')).toBeVisible()
  await page.getByRole('button', { name: 'Batal' }).click()
})

test('Pilot: shadow mode chip, dashboard per agent + audit; Konfirmasi share of wallet (stage 14)', async ({ page, request }) => {
  await request.post('/api/pilot/mode', { data: { mode: 'shadow', branch: 'Semarang' } })
  try {
    await page.goto('/pengaturan/pilot', { waitUntil: 'networkidle' })
    await expect(page.locator('.chip-sample').filter({ hasText: 'Mode bayangan' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Pilot cabang Semarang' })).toBeVisible()
    await expect(page.locator('.tbl tbody tr')).toHaveCount(6)
    await expect(page.getByText('Kirim selama mode bayangan')).toBeVisible()
    await page.goto('/dealer/sinar', { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: 'Konfirmasi share of wallet' }).click()
    await expect(page.locator('.sheet .tbl tbody tr').first()).toBeVisible()
    await page.getByRole('button', { name: 'Batal' }).click()
  } finally {
    await request.post('/api/pilot/mode', { data: { mode: 'off' } })
  }
})
