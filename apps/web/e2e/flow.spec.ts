import { expect, test } from '@playwright/test'
import { API, login, postWaEvent, trackErrors } from './helpers'

// Stage 13 end-to-end: WA in → analysis → opportunity → approve reply → sent (Fake/real) → Odoo note → Kas.
// Mutates the database: run `make reset` before and after.
test('WhatsApp inbound becomes a lead, a reply is sent only after human approval', async ({ page }) => {
  const errors = trackErrors(page)
  await login(page)
  const req = page.request
  const suffix = String(Date.now()).slice(-6)
  const phone = '6281277' + suffix
  const chat = phone + '@s.whatsapp.net'
  const first = `Selamat siang Pak Andi, saya Budi dari PT Sinar Logistik ${suffix}. Kami butuh penawaran 16 kamera CCTV untuk gudang Sidoarjo, PO bisa Kamis.`

  // 1. Unknown number writes in → InboundContact (message not stored as chat until identified/lead).
  const r1 = await postWaEvent(req, { wamid: 'E2E-A-' + suffix, session: 's-andi', from: phone, to: '6281200000001', chat_id: chat, sender_name: 'Budi Santoso', text: first })
  expect(r1.results[0].inbound_id).toBeTruthy()
  const inboundId: string = r1.results[0].inbound_id

  // 2. Identity agent runs; the prospect shows up in Prospek with its first message.
  expect((await req.post(API + '/api/jobs/identify_inbound/run')).ok()).toBeTruthy()
  await page.goto('/#pros')
  await expect(page.locator('main.stage')).toContainText('PT Sinar Logistik ' + suffix)

  // 3. Human turns it into a lead → opportunity with the questions attached.
  const lead = await req.post(API + '/api/prospects/' + inboundId + '/lead')
  expect(lead.ok(), await lead.text()).toBeTruthy()
  const opps = await (await req.get(API + '/api/prospects/' + inboundId)).json()
  expect(opps.lead).toBe(true)

  // 4. Follow-up message is now stored in a customer thread.
  const r2 = await postWaEvent(req, { wamid: 'E2E-B-' + suffix, session: 's-andi', from: phone, to: '6281200000001', chat_id: chat, sender_name: 'Budi Santoso', text: 'Pak, sekalian minta brosur NVR 32 channel ya.' })
  const threadId: string = r2.results[0].thread_id
  expect(threadId).toBeTruthy()

  // 5. Reply from Chat → only a proposed action, nothing sent.
  await page.goto('/#chat/' + threadId)
  await expect(page.locator('main.stage')).toContainText('brosur NVR 32 channel')
  await page.fill('#chat-input', 'Siap Pak Budi, brosur dan penawaran kami kirim hari ini.')
  await page.keyboard.press('Enter')
  await expect(page.locator('.toast')).toContainText('antrean persetujuan')
  const before = await (await req.get(API + '/api/chat/threads/' + threadId + '/messages')).text()
  expect(before).not.toContain('terkirim')

  // 6. Human approves in Hari ini → sent through the transport (Fake unless the number is linked to the bridge).
  const list = await (await req.get(API + '/api/actions?status=proposed')).json()
  const items = Array.isArray(list) ? list : list.items
  const action = items.find((a: { title: string }) => a.title.includes('Kirim balasan ke'))
  expect(action, 'reply action in queue').toBeTruthy()
  await page.goto('/#today')
  const card = page.locator('#q-' + action.id)
  await expect(card).toBeVisible()
  await card.locator('.q-quick button').click()
  await expect(page.locator('.toast')).toContainText('Terkirim')
  const after = await (await req.get(API + '/api/chat/threads/' + threadId + '/messages')).text()
  expect(after).toContain('Siap Pak Budi')

  // 7. Kas still renders (L2C pipeline continues from Odoo).
  await page.goto('/#cash')
  await expect(page.locator('main.stage')).toContainText('Rp')
  expect(errors).toEqual([])
})
