import { describe, expect, it } from 'vitest'
import { defaultPolicy } from '../src/config.js'
import { SendError, Sender, type SendTarget } from '../src/sender.js'
import { MemoryStore } from '../src/store.js'

const P = { ...defaultPolicy, typingMinMs: 0, typingMaxMs: 0, typingMsPerChar: 0 }
const office = () => new Date(Date.UTC(2026, 9, 5, 3, 0)) // 10:00 WIB

function target(connected = true) {
  const sent: { chat: string; text: string; edit?: string }[] = []
  const t: SendTarget = {
    id: 's1',
    connected: () => connected,
    sendText: async (chat, text, _typing, quote) => { sent.push({ chat, text, ...(quote ? { edit: 'quote:' + quote.id } : {}) }); return 'WAMID' + sent.length },
    editText: async (chat, wamid, text) => { sent.push({ chat, text, edit: wamid }) },
    deleteText: async (chat, wamid) => { sent.push({ chat, text: '', edit: 'delete:' + wamid }) },
  }
  return { t, sent }
}

async function setup(approved: string[] = ['a1', 'a2', 'a3', 'a4', 'a5']) {
  const store = new MemoryStore()
  await store.upsertSession({ id: 's1', label: 'S1', historyDays: 30, pairedAt: new Date(Date.UTC(2026, 0, 1)), phone: '628111' })
  await store.markInbound('s1', '6281@s.whatsapp.net', new Date(), false)
  const sleeps: number[] = []
  const sender = new Sender(store, { actionApproved: async id => approved.includes(id) }, P, { now: office, sleep: async ms => { sleeps.push(ms) }, rnd: () => 0 })
  return { store, sender, sleeps }
}

const code = async (p: Promise<unknown>) => p.then(() => 'ok', (e: SendError) => `${e.status}:${e.code}`)

describe('sender', () => {
  it('sends only actions the API confirms as human-approved', async () => {
    const { sender } = await setup(['a1'])
    const { t, sent } = target()
    expect(await code(sender.send(t, '6281@s.whatsapp.net', 'Halo', ''))).toBe('403:not_approved')
    expect(await code(sender.send(t, '6281@s.whatsapp.net', 'Halo', 'proposed-9'))).toBe('403:not_approved')
    expect(sent).toHaveLength(0)
    expect(await sender.send(t, '6281@s.whatsapp.net', 'Halo', 'a1')).toEqual({ wamid: 'WAMID1', duplicate: false })
  })
  it('deletes for everyone only for an approved action, once', async () => {
    const { sender } = await setup(['a1'])
    const { t, sent } = target()
    expect(await code(sender.revoke(t, '6281@s.whatsapp.net', 'W9', 'nope'))).toBe('403:not_approved')
    await sender.revoke(t, '6281@s.whatsapp.net', 'W9', 'a1')
    expect(await sender.revoke(t, '6281@s.whatsapp.net', 'W9', 'a1')).toEqual({ wamid: 'W9', duplicate: true })
    expect(sent).toEqual([{ chat: '6281@s.whatsapp.net', text: '', edit: 'delete:W9' }])
  })
  it('sends a reply quoting the message it answers', async () => {
    const { sender } = await setup(['a1'])
    const { t, sent } = target()
    await sender.send(t, '6281@s.whatsapp.net', 'Siap Pak', 'a1', { id: 'Q9', from_me: false, text: 'Ada stok DVR?' })
    expect(sent).toEqual([{ chat: '6281@s.whatsapp.net', text: 'Siap Pak', edit: 'quote:Q9' }])
  })
  it('edits only for approved actions, once, without the hourly caps', async () => {
    const { sender, store } = await setup(['a1'])
    const { t, sent } = target()
    expect(await code(sender.edit(t, '6281@s.whatsapp.net', 'W9', 'Halo Pak', 'nope'))).toBe('403:not_approved')
    expect(await code(sender.edit(t, '6281@s.whatsapp.net', '', 'Halo Pak', 'a1'))).toBe('400:bad_request')
    expect(await sender.edit(t, '6281@s.whatsapp.net', 'W9', 'Halo Pak', 'a1')).toEqual({ wamid: 'W9', duplicate: false })
    expect(await sender.edit(t, '6281@s.whatsapp.net', 'W9', 'Halo Pak', 'a1')).toEqual({ wamid: 'W9', duplicate: true })
    expect(sent).toEqual([{ chat: '6281@s.whatsapp.net', text: 'Halo Pak', edit: 'W9' }])
    expect(store.sent).toHaveLength(1)
    expect(await code(sender.edit(target(false).t, '6281@s.whatsapp.net', 'W9', 'x', 'a2'))).toBe('403:not_approved')
  })
  it('never sends the same action twice (retried request)', async () => {
    const { sender } = await setup()
    const { t, sent } = target()
    await sender.send(t, '6281@s.whatsapp.net', 'Halo', 'a1')
    expect(await sender.send(t, '6281@s.whatsapp.net', 'Halo', 'a1')).toEqual({ wamid: 'WAMID1', duplicate: true })
    expect(sent).toHaveLength(1)
  })
  it('refuses unlinked sessions and cold contacts', async () => {
    const { sender } = await setup()
    expect(await code(sender.send(target(false).t, '6281@s.whatsapp.net', 'Halo', 'a1'))).toBe('409:not_linked')
    expect(await code(sender.send(target().t, '6299@s.whatsapp.net', 'Halo', 'a1'))).toBe('403:first_contact')
  })
  it('honours opt-out and re-opens when the contact writes again', async () => {
    const { sender, store } = await setup()
    const { t } = target()
    await store.markInbound('s1', '6281@s.whatsapp.net', new Date(), true)
    expect(await code(sender.send(t, '6281@s.whatsapp.net', 'Halo', 'a1'))).toBe('403:opted_out')
    await store.markInbound('s1', '6281@s.whatsapp.net', new Date(), false)
    expect(await code(sender.send(t, '6281@s.whatsapp.net', 'Halo', 'a1'))).toBe('ok')
  })
  it('waits a random human gap between consecutive sends and spaces the same chat', async () => {
    const { sender, sleeps, store } = await setup()
    await store.markInbound('s1', '6282@s.whatsapp.net', new Date(), false)
    const { t } = target()
    await sender.send(t, '6281@s.whatsapp.net', 'Pesan satu', 'a1')
    await sender.send(t, '6282@s.whatsapp.net', 'Pesan dua', 'a2')
    expect(sleeps).toEqual([2000])
    expect(await code(sender.send(t, '6281@s.whatsapp.net', 'Pesan tiga', 'a3'))).toBe('429:chat_gap')
  })
  it('blocks the same text going to many chats', async () => {
    const { sender, store } = await setup()
    const { t } = target()
    for (const n of [1, 2, 3, 4]) await store.markInbound('s1', `62${n}@s.whatsapp.net`, new Date(), false)
    for (const [i, n] of [1, 2, 3].entries()) await sender.send(t, `62${n}@s.whatsapp.net`, 'Promo akhir bulan!', 'a' + (i + 1))
    expect(await code(sender.send(t, '624@s.whatsapp.net', 'Promo  akhir bulan!', 'a4'))).toBe('403:broadcast')
  })
  it('refuses during quiet hours with 409', async () => {
    const store = new MemoryStore()
    await store.markInbound('s1', '6281@s.whatsapp.net', new Date(), false)
    const night = () => new Date(Date.UTC(2026, 9, 5, 15, 0)) // 22:00 WIB
    const sender = new Sender(store, { actionApproved: async () => true }, P, { now: night, sleep: async () => {} })
    expect(await code(sender.send(target().t, '6281@s.whatsapp.net', 'Halo', 'a1'))).toBe('409:quiet_hours')
  })
})
