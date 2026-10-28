import { describe, expect, it } from 'vitest'
import { callLine, contactCard, deletableNow, editableNow, fmtDuration, fmtSize, forwardable, mediaCaption, mediaName } from './bubble'

describe('bubble media and edits', () => {
  it('shows the caption without the media label', () => {
    expect(mediaCaption('[gambar] kamera rusak', { kind: 'image', viewable: true })).toBe('kamera rusak')
    expect(mediaCaption('[dokumen BoQ.pdf]', { kind: 'document', viewable: true })).toBe('')
    expect(mediaCaption('[bukan media] teks', undefined)).toBe('[bukan media] teks')
  })
  it('names media like WhatsApp', () => {
    expect(mediaName({ kind: 'audio', ptt: true, viewable: true })).toBe('Pesan suara')
    expect(mediaName({ kind: 'document', file_name: 'BoQ.pdf', viewable: true })).toBe('BoQ.pdf')
    expect(mediaName({ kind: 'video', viewable: true })).toBe('Video')
    expect(fmtDuration(7)).toBe('0:07')
    expect(fmtDuration(92)).toBe('1:32')
    expect(fmtSize(123_000)).toBe('120 KB')
    expect(fmtSize(3_565_158)).toBe('3,4 MB')
  })
  it('offers edit for 15 minutes after sending, and while queued', () => {
    const now = new Date('2026-10-13T05:00:00Z')
    expect(editableNow({ can_edit: true, status: 'sent', sent_at: '2026-10-13T04:50:00Z' }, now)).toBe(true)
    expect(editableNow({ can_edit: true, status: 'sent', sent_at: '2026-10-13T04:44:00Z' }, now)).toBe(false)
    expect(editableNow({ can_edit: true, status: 'pending', sent_at: '2026-10-13T04:00:00Z' }, now)).toBe(true)
    expect(editableNow({ can_edit: false, status: 'sent', sent_at: '2026-10-13T04:59:00Z' }, now)).toBe(false)
  })
})

describe('bubble delete', () => {
  it('offers delete for everyone for 48 hours, and while queued', () => {
    const now = new Date('2026-10-13T05:00:00Z')
    expect(deletableNow({ can_delete: true, status: 'sent', sent_at: '2026-10-12T05:00:00Z' }, now)).toBe(true)
    expect(deletableNow({ can_delete: true, status: 'sent', sent_at: '2026-10-11T04:00:00Z' }, now)).toBe(false)
    expect(deletableNow({ can_delete: true, status: 'pending', sent_at: '2026-10-01T00:00:00Z' }, now)).toBe(true)
    expect(deletableNow({ can_delete: false, status: 'sent', sent_at: '2026-10-13T04:59:00Z' }, now)).toBe(false)
  })
})

describe('bubble forward', () => {
  it('forwards text and files WhatsApp still holds, not a sending message, a poll or a lost file', () => {
    expect(forwardable({ body: 'Stok IPC ready', status: 'received' })).toBe(true)
    expect(forwardable({ body: '[gambar] Lokasi', status: 'received', media: { kind: 'image', viewable: true } })).toBe(true)
    expect(forwardable({ body: '[gambar]', status: 'received', media: { kind: 'image', viewable: false } })).toBe(false)
    expect(forwardable({ body: '[polling] Kirim kapan?', status: 'sent', media: { kind: 'poll', viewable: true } })).toBe(false)
    expect(forwardable({ body: 'halo', status: 'pending' })).toBe(false)
    expect(forwardable({ body: 'halo', status: 'received', deleted: true })).toBe(false)
  })
})

describe('contact card', () => {
  it('reads the name and number of a card bubble', () => {
    expect(contactCard('Pak Budi · +6281234567890')).toEqual({ name: 'Pak Budi', phone: '+6281234567890' })
    expect(contactCard('Bu Sari')).toEqual({ name: 'Bu Sari', phone: '' })
  })
})

describe('callLine', () => {
  it('words a call bubble and marks a missed one (ADR 0062)', () => {
    expect(callLine({ kind: 'call', call: 'missed', viewable: false })).toEqual({ text: 'Panggilan suara tak terjawab', tone: 'bad' })
    expect(callLine({ kind: 'call', call: 'rejected', video: true, viewable: false })).toEqual({ text: 'Panggilan video ditolak', tone: 'bad' })
    expect(callLine({ kind: 'call', call: 'answered', seconds: 192, viewable: false })).toEqual({ text: 'Panggilan suara · 3:12', tone: 'good' })
    expect(callLine({ kind: 'call', call: 'answered', viewable: false }).text).toBe('Panggilan suara · diangkat di HP')
  })
})
