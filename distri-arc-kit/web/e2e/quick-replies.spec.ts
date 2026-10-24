import { expect, test } from '@playwright/test'

// Balasan cepat (ADR 0065): "/" in the composer lists the team's saved texts; a pick fills the box, nothing is sent.
test('balasan cepat: save one, type / and pick it into the box', async ({ page, request }) => {
  const sc = 'e2e-' + Date.now().toString(36)
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/chat', { waitUntil: 'networkidle' })
  await page.locator('.ibr').first().click()
  const box = page.getByLabel('Tulis balasan')
  await expect(box).toBeVisible()

  // save one from the sheet (📎 → Balasan cepat)
  await page.getByLabel('Lampirkan').click()
  await page.getByRole('menuitem', { name: 'Balasan cepat' }).click()
  await page.getByLabel('Pintasan').fill('/' + sc)
  await page.getByLabel('Isi pesan').fill('Transfer ke BCA 123 a.n. PT GSI')
  await page.getByRole('button', { name: 'Tambah' }).click()
  await expect(page.locator('.qr-item b', { hasText: '/' + sc })).toBeVisible()
  await page.screenshot({ path: 'e2e/__screenshots__/quick-replies-sheet.png' })
  await page.keyboard.press('Escape')

  // type "/" + part of the shortcut: the list shows it, Enter fills the box
  await box.click()
  await box.fill('Baik bu ')
  await box.pressSequentially('/' + sc.slice(0, 6))
  const opt = page.getByRole('option', { name: new RegExp('/' + sc) })
  await expect(opt).toBeVisible()
  await page.screenshot({ path: 'e2e/__screenshots__/quick-replies-picker.png' })
  await box.press('Enter')
  await expect(box).toHaveValue('Baik bu Transfer ke BCA 123 a.n. PT GSI')
  await expect(page.getByRole('listbox', { name: 'Balasan cepat' })).toHaveCount(0)
  expect(errors).toEqual([])

  // clean up
  const list = await (await request.get('/api/chat/quick-replies')).json()
  for (const q of list.items) if (q.shortcut === sc) await request.delete('/api/chat/quick-replies/' + q.id)
})
