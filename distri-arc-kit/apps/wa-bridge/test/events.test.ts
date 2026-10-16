import type { WAMessage } from 'baileys'
import { describe, expect, it } from 'vitest'
import { decodeMediaRef, messageText, toWaEvent, type Resolver } from '../src/events.js'

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
  it('passes on the members a group message tags, a LID resolved to the phone number', async () => {
    const ev = await toWaEvent('s-andi', msg({ id: 'T1', remoteJid: '1203@g.us', participant: '111@lid', fromMe: false }, {
      extendedTextMessage: { text: '@6281200000001 tolong cek @222 juga', contextInfo: { mentionedJid: ['6281200000001@s.whatsapp.net', '222@lid', '222@lid'] } },
    }), r, false)
    expect(ev?.mentions).toEqual([{ token: '6281200000001', phone: '6281200000001', name: '' }, { token: '222', phone: '', name: '' }])
    const plain = await toWaEvent('s-andi', msg({ id: 'T2', remoteJid: '1203@g.us', participant: '111@lid', fromMe: false }, { conversation: 'tanpa tag' }), r, false)
    expect(plain?.mentions).toBeUndefined()
  })
  it('keeps a download reference for media: key and CDN fields only, no caption or thumbnail', async () => {
    const key = new Uint8Array([1, 2, 3, 4])
    const ev = await toWaEvent('s', msg({ id: 'IMG1', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false }, {
      imageMessage: { caption: 'Foto kamera rusak', mimetype: 'image/jpeg', fileLength: 2048, mediaKey: key, directPath: '/v/t62/abc', url: 'https://mmg.whatsapp.net/abc', jpegThumbnail: new Uint8Array(500) },
    }), r, false)
    expect(ev).toMatchObject({ text: 'Foto kamera rusak', media_meta: { kind: 'image', mime: 'image/jpeg', size: 2048 } })
    const back = decodeMediaRef(ev!.media_meta!.ref!)
    expect(back.key).toMatchObject({ id: 'IMG1', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false })
    expect(back.message?.imageMessage).toMatchObject({ directPath: '/v/t62/abc', mimetype: 'image/jpeg' })
    expect([...(back.message?.imageMessage?.mediaKey ?? [])]).toEqual([1, 2, 3, 4])
    expect(back.message?.imageMessage?.caption ?? '').toBe('')
    expect(back.message?.imageMessage?.jpegThumbnail?.length ?? 0).toBe(0)
    const vn = await toWaEvent('s', msg({ id: 'VN1', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false }, { audioMessage: { mimetype: 'audio/ogg; codecs=opus', seconds: 12, ptt: true, mediaKey: key, directPath: '/v/a' } }), r, false)
    expect(vn?.media_meta).toMatchObject({ kind: 'audio', seconds: 12, ptt: true })
    expect(vn?.media_meta?.ref).toBeTruthy()
  })
  it('keeps the message a reply quotes, for text and media alike', async () => {
    const ctx = { stanzaId: 'Q1', participant: '6281299887766@s.whatsapp.net' }
    expect(messageText({ extendedTextMessage: { text: 'Siap', contextInfo: ctx } }).quoted).toBe('Q1')
    expect(messageText({ audioMessage: { mimetype: 'audio/ogg', contextInfo: ctx } }).quoted).toBe('Q1')
    expect(messageText({ documentMessage: { fileName: 'PO.pdf', contextInfo: ctx } }).quoted).toBe('Q1')
    expect(messageText({ conversation: 'tanpa kutipan' }).quoted).toBeUndefined()
  })
  it('maps a delete for everyone to the deleted message id', async () => {
    const ev = await toWaEvent('s', msg({ id: 'D1', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false }, {
      protocolMessage: { type: 0, key: { id: 'ORIG2', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false } },
    }), r, false)
    expect(ev).toMatchObject({ wamid: 'D1', revoke_of: 'ORIG2', text: '' })
  })
  it('maps an edit to the edited message id and its new text', async () => {
    const ev = await toWaEvent('s', msg({ id: 'E1', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false }, {
      protocolMessage: { type: 14, key: { id: 'ORIG1', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false }, editedMessage: { conversation: 'Minta 20 kamera, bukan 16' } },
    }), r, false)
    expect(ev).toMatchObject({ wamid: 'E1', edit_of: 'ORIG1', text: 'Minta 20 kamera, bukan 16' })
  })
  it('keeps media metadata but never content, and skips status, reactions and protocol messages', async () => {
    expect(messageText({ documentMessage: { fileName: 'BoQ.pdf', mimetype: 'application/pdf', caption: 'BoQ revisi' } })).toMatchObject({ text: 'BoQ revisi', media: { kind: 'document', file_name: 'BoQ.pdf' } })
    expect(await toWaEvent('s', msg({ id: 'S', remoteJid: 'status@broadcast', fromMe: false }, { conversation: 'status' }), r, false)).toBeNull()
    expect(await toWaEvent('s', msg({ id: 'R', remoteJid: '628@s.whatsapp.net', fromMe: false }, { reactionMessage: { text: '👍' } }), r, false)).toBeNull()
    expect(await toWaEvent('s', msg({ id: 'P', remoteJid: '628@s.whatsapp.net', fromMe: false }, { protocolMessage: {} }), r, false)).toBeNull()
  })
})
