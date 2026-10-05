import { expect, test } from 'vitest'
import { fmtRp, fx1, todayLine, shortName, contactIni } from './format'

test('fmtRp matches the mockup', () => {
  expect(fmtRp(62e6)).toBe('Rp 62 jt')
  expect(fmtRp(18.4e6)).toBe('Rp 18 jt')
  expect(fmtRp(1.28e9)).toBe('Rp 1,28 M')
  expect(fmtRp(1.02e9)).toBe('Rp 1,02 M')
  expect(fmtRp(1e9)).toBe('Rp 1 M')
  expect(fmtRp(2.5e9)).toBe('Rp 2,5 M')
  expect(fmtRp(0)).toBe('Rp 0 jt')
})

test('helpers', () => {
  expect(fx1(2.142857)).toBe('2,1')
  expect(todayLine('2026-10-05T06:45:00+07:00')).toBe('Senin, 5 Oktober 2026 · minggu pertama Q4')
  expect(shortName('CV Mitra Jaya Teknik')).toBe('Mitra Jaya Teknik')
  expect(contactIni('Pak Budi')).toBe('BU')
})
