import type { WAMessage } from 'baileys'
import { describe, expect, it } from 'vitest'
import { messageText, toWaEvent, type Resolver } from '../src/events.js'

const r: Resolver = {
  ownPhone: '6281200000001',
  pnForLid: async lid => (lid === '111@lid' ? '6281299887766@s.whatsapp.net' : null),
  groupMeta: async () => ({ name: 'Proyek RSUD', members: [{ jid: '6281299887766@s.whatsapp.net', phone: '6281299887766', name: 'Budi' }] }),
}

const msg = (key: WAMessage['key'], message: WAMessage['message'], extra: Partial<WAMessage> = {}): WAMessage =>
  ({ key, message, messageTimestamp: 1_790_000_000, pushName: 'Budi Santoso', ...extra }) as WAMessage

describe('Baileys → WaEvent', () => {
  it('maps a customer 1:1 message, resolving LID chats to phone numbers', async () => {
    const ev = await toWaEvent('s-andi', msg({ id: 'A1', remoteJid: '111@lid', fromMe: false }, { conversation: 'Minta penawaran 16 kamera' }), r, false)
    expect(ev).toMatchObject({ wamid: 'A1', session: 's-andi', chat_id: '6281299887766@s.whatsapp.net', from: '6281299887766', to: '6281200000001',
      is_group: false, sender_name: 'Budi Santoso', text: 'Minta penawaran 16 kamera', from_me: false, is_history: false, transport: 'bridge' })
  })
  it('maps own messages sent from the phone and group messages with metadata', async () => {
    const own = await toWaEvent('s-andi', msg({ id: 'A2', remoteJid: '6281299887766@s.whatsapp.net', fromMe: true }, { extendedTextMessage: { text: 'Siap Pak' } }), r, true)
    expect(own).toMatchObject({ from: '6281200000001', to: '6281299887766', from_me: true, is_history: true, sender_name: '' })
    const g = await toWaEvent('s-andi', msg({ id: 'A3', remoteJid: '1203@g.us', participant: '111@lid', fromMe: false }, { conversation: 'Jadwal survei Kamis' }), r, false)
    expect(g).toMatchObject({ is_group: true, chat_id: '1203@g.us', from: '6281299887766', group_meta: { name: 'Proyek RSUD' } })
  })
  it('keeps media metadata but never content, and skips status, reactions and protocol messages', async () => {
    expect(messageText({ documentMessage: { fileName: 'BoQ.pdf', mimetype: 'application/pdf', caption: 'BoQ revisi' } })).toMatchObject({ text: 'BoQ revisi', media: { kind: 'document', file_name: 'BoQ.pdf' } })
    expect(await toWaEvent('s', msg({ id: 'S', remoteJid: 'status@broadcast', fromMe: false }, { conversation: 'status' }), r, false)).toBeNull()
    expect(await toWaEvent('s', msg({ id: 'R', remoteJid: '628@s.whatsapp.net', fromMe: false }, { reactionMessage: { text: '👍' } }), r, false)).toBeNull()
    expect(await toWaEvent('s', msg({ id: 'P', remoteJid: '628@s.whatsapp.net', fromMe: false }, { protocolMessage: {} }), r, false)).toBeNull()
  })
})
