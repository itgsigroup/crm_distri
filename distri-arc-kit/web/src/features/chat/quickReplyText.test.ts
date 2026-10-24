import { describe, expect, it } from 'vitest'
import type { QuickReply } from '../../api/types'
import { cleanShortcut, insertQuick, matchQuick, slashQuery, validShortcut } from './quickReplyText'

const qr = (shortcut: string, body: string): QuickReply => ({ id: shortcut, shortcut, body, by: 'Andi', uses: 0, updated_at: '', can_edit: true })
const items = [qr('salam', 'Selamat pagi, ada yang bisa kami bantu?'), qr('rekening', 'Transfer ke BCA 123'), qr('rek-mandiri', 'Mandiri 456'), qr('alamat', 'Gudang di Jl. Rekso 1')]

describe('balasan cepat', () => {
  it('opens on "/" at the start of a word, not inside one', () => {
    expect(slashQuery('/', 1)).toEqual({ start: 0, q: '' })
    expect(slashQuery('/rek', 4)).toEqual({ start: 0, q: 'rek' })
    expect(slashQuery('Baik bu /rek', 12)).toEqual({ start: 8, q: 'rek' })
    expect(slashQuery('Baik\n/sal', 9)).toEqual({ start: 5, q: 'sal' })
    expect(slashQuery('1/2 lusin', 2)).toBeNull()
    expect(slashQuery('http://x', 7)).toBeNull()
    expect(slashQuery('/rek ', 5)).toBeNull() // a space closes it
  })
  it('lists shortcuts that start with the text first, then the rest that contain it', () => {
    expect(matchQuick(items, '').map((x) => x.shortcut)).toEqual(['salam', 'rekening', 'rek-mandiri', 'alamat'])
    expect(matchQuick(items, 'REK').map((x) => x.shortcut)).toEqual(['rekening', 'rek-mandiri', 'alamat'])
    expect(matchQuick(items, 'mandiri').map((x) => x.shortcut)).toEqual(['rek-mandiri'])
    expect(matchQuick(items, 'zzz')).toEqual([])
  })
  it('puts the text in place of "/…" and keeps what is around it', () => {
    expect(insertQuick('Baik bu /rek', { start: 8, q: 'rek' }, 'Transfer ke BCA 123')).toEqual({ text: 'Baik bu Transfer ke BCA 123', caret: 27 })
    expect(insertQuick('/sa terima kasih', { start: 0, q: 'sa' }, 'Halo')).toEqual({ text: 'Halo terima kasih', caret: 4 })
  })
  it('cleans and checks a shortcut like the API', () => {
    expect(cleanShortcut(' /Rekening BCA ')).toBe('rekening-bca')
    expect(validShortcut('rekening-bca')).toBe(true)
    expect(validShortcut('')).toBe(false)
    expect(validShortcut('-x')).toBe(false)
    expect(validShortcut('x'.repeat(25))).toBe(false)
  })
})
