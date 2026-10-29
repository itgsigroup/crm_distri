import { describe, expect, it } from 'vitest'
import { daysText, fill, windowsError } from './autoreply'

describe('chat otomatis', () => {
  it('fills the template as the server does', () => {
    expect(fill('{salam} {nama}, kembali {kembali} WIB. (Pesan otomatis)', 'Toko Sinar', '13.00', 'siang'))
      .toBe('Selamat siang Toko Sinar, kembali 13.00 WIB. (Pesan otomatis)')
  })

  it('checks the hours inside the send window', () => {
    const ok = { days: [1, 2], start: '12:00', end: '13:00' }
    expect(windowsError([ok], '08:00', '18:00')).toBe('')
    expect(windowsError([], '08:00', '18:00')).not.toBe('')
    expect(windowsError([{ ...ok, start: '13:00', end: '12:00' }], '08:00', '18:00')).toMatch(/setelah mulai/)
    expect(windowsError([{ ...ok, end: '19:00' }], '08:00', '18:00')).toMatch(/08:00–18:00/)
    expect(windowsError([{ ...ok, days: [] }], '08:00', '18:00')).toMatch(/hari/)
  })

  it('names the days', () => {
    expect(daysText([1, 2, 3, 4, 5, 6])).toBe('Sen–Sab')
    expect(daysText([5, 1, 3])).toBe('Sen, Rab, Jum')
    expect(daysText([1, 2, 3, 4, 5, 6, 7])).toBe('Setiap hari')
  })
})
