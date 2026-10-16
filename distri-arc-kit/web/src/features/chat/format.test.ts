import { describe, expect, it } from 'vitest'
import { formatParts, quoteLines, shortcutMark, wrapSelection } from './format'

describe('WhatsApp formatting', () => {
  it('reads bold, italic, strikethrough and monospace, nested too', () => {
    expect(formatParts('harga *Rp 1.500.000* _belum PPN_ ~lama~')).toEqual([
      { t: 'harga ' }, { t: 'Rp 1.500.000', b: true }, { t: ' ' }, { t: 'belum PPN', i: true }, { t: ' ' }, { t: 'lama', s: true },
    ])
    expect(formatParts('*_penting_*')).toEqual([{ t: 'penting', b: true, i: true }])
    expect(formatParts('kode ```IPC-*4MP*``` ya')).toEqual([{ t: 'kode ' }, { t: 'IPC-*4MP*', m: true }, { t: ' ya' }])
    expect(formatParts('`sn123`.')).toEqual([{ t: 'sn123', m: true }, { t: '.' }])
  })
  it('leaves markers that WhatsApp would not format', () => {
    for (const s of ['2*3*4', 'a * b * c', 'file_nama_baru.pdf', 'https://a.id/x_y_z', '*tanpa tutup', '* spasi*', '*baris\nbaru*']) {
      expect(formatParts(s)).toEqual([{ t: s }])
    }
  })
  it('marks quote lines', () => {
    expect(quoteLines('> stok ada?\nada')).toEqual([{ t: 'stok ada?', quote: true }, { t: 'ada', quote: false }])
  })
  it('wraps and unwraps the selection, spaces outside', () => {
    expect(wrapSelection('kirim besok ', 6, 12, '*')).toEqual({ text: 'kirim *besok* ', start: 7, end: 12 })
    expect(wrapSelection('kirim *besok* ', 7, 12, '*')).toEqual({ text: 'kirim besok ', start: 6, end: 11 })
    expect(wrapSelection('kirim *besok*', 6, 13, '*')).toEqual({ text: 'kirim besok', start: 6, end: 11 })
    expect(wrapSelection('ok ', 3, 3, '_')).toEqual({ text: 'ok __', start: 4, end: 4 })
    expect(wrapSelection('SN', 0, 2, '```')).toEqual({ text: '```SN```', start: 3, end: 5 })
  })
  it('knows WhatsApp Web shortcuts', () => {
    const k = (key: string, shiftKey = false, metaKey = false) => shortcutMark({ key, ctrlKey: !metaKey, metaKey, shiftKey, altKey: false })
    expect([k('b'), k('i'), k('X', true), k('M', true), k('b', false, true), k('c')]).toEqual(['*', '_', '~', '```', '*', null])
  })
})
