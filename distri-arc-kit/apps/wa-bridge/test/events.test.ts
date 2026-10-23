import type { WAMessage } from 'baileys'
import { describe, expect, it } from 'vitest'
import { decodeMediaRef, groupStubLine, messageText, stubParticipants, toGroupMeta, toWaEvent, type Resolver } from '../src/events.js'

const r: Resolver = {
  ownPhone: '6281200000001',
  pnForLid: async lid => (lid === '111@lid' ? '6281299887766@s.whatsapp.net' : null),
  groupMeta: async () => ({ name: 'Proyek RSUD', members: [{ jid: '6281299887766@s.whatsapp.net', phone: '6281299887766', name: 'Budi' }] }),
}

const msg = (key: WAMessage['key'], message: WAMessage['message'], extra: Partial<WAMessage> = {}): WAMessage =>
  ({ key, message, messageTimestamp: 1_790_000_000, pushName: 'Budi Santoso', ...extra }) as WAMessage

describe('Baileys → WaEvent', () => {
  it('turns a group stub (created, member added) into a system line so the chat exists before its first text', async () => {
    const created = await toWaEvent('s-andi', msg({ id: 'G1', remoteJid: '1203@g.us', participant: '6281200000001@s.whatsapp.net', fromMe: false },
      null, { messageStubType: 20, messageStubParameters: ['Proyek RSUD'], pushName: '' }), r, false)
    expect(created).toMatchObject({ wamid: 'G1', chat_id: '1203@g.us', is_group: true, from_me: true, from: '6281200000001', sender_name: '',
      text: 'Anda membuat grup "Proyek RSUD"', media_meta: { kind: 'system', system: 'create' }, group_meta: { name: 'Proyek RSUD' } })
    const added = await toWaEvent('s-andi', msg({ id: 'G2', remoteJid: '1203@g.us', participant: '111@lid', fromMe: false },
      null, { messageStubType: 27, messageStubParameters: ['{"id":"6281200000001@s.whatsapp.net"}', '{"id":"555@lid","phoneNumber":"6285500001111@s.whatsapp.net"}'] }), r, false)
    expect(added).toMatchObject({ from: '6281299887766', from_me: false, text: 'Budi menambahkan Anda, +6285500001111', media_meta: { kind: 'system', system: 'add' } })
    // history has them by the hundred: the group is known by then
    expect(await toWaEvent('s-andi', msg({ id: 'G3', remoteJid: '1203@g.us', fromMe: false }, null, { messageStubType: 20, messageStubParameters: ['X'] }), r, true)).toBeNull()
    // other stubs (e2e notices, ephemeral settings) and 1:1 stubs stay out
    expect(await toWaEvent('s-andi', msg({ id: 'G4', remoteJid: '1203@g.us', fromMe: false }, null, { messageStubType: 2 }), r, false)).toBeNull()
    expect(await toWaEvent('s-andi', msg({ id: 'G5', remoteJid: '6281299887766@s.whatsapp.net', fromMe: false }, null, { messageStubType: 20 }), r, false)).toBeNull()
  })
  it('words group changes as WhatsApp does and reads participants from JSON or bare JIDs', () => {
    expect(groupStubLine('leave', 'Anda', ['Budi'], [])).toBe('Budi keluar')
    expect(groupStubLine('subject', 'Rina', [], ['Tim Baru'])).toBe('Rina mengubah nama grup menjadi "Tim Baru"')
    expect(groupStubLine('create', 'Anda', [], [])).toBe('Anda membuat grup')
    expect(stubParticipants(['6281200000001@s.whatsapp.net', '{"id":"555@lid"}', 'junk'])).toEqual([
      { jid: '6281200000001@s.whatsapp.net', phone: '6281200000001' }, { jid: '555@lid', phone: '' }])
  })
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
  it('lists group members with their numbers, names from the phone and admins', () => {
    const g = toGroupMeta('Proyek RSUD', [
      { id: '6281299887766@s.whatsapp.net', admin: 'superadmin' },
      { id: '333@lid', phoneNumber: '6281511112222@s.whatsapp.net', notify: 'Rina' },
      { id: '6281533334444@s.whatsapp.net', notify: 'Profil Dodi' },
      { id: '444@lid' },
    ], (_jid, phone) => ({ '6281299887766': 'Budi Toko', '6281533334444': 'Pak Dodi (disimpan)' })[phone] ?? '')
    expect(g.members).toEqual([
      { jid: '6281299887766@s.whatsapp.net', phone: '6281299887766', name: 'Budi Toko', admin: true },
      { jid: '333@lid', phone: '6281511112222', name: 'Rina' },
      { jid: '6281533334444@s.whatsapp.net', phone: '6281533334444', name: 'Pak Dodi (disimpan)' },
      { jid: '444@lid', phone: '', name: '' },
    ])
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
