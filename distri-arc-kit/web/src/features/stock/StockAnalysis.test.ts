import { describe, expect, it } from 'vitest'
import { coverText, rpShort } from './StockAnalysis'

describe('Analisa stok labels', () => {
  it('shortens rupiah', () => {
    expect(rpShort(850_000)).toBe('Rp 850.000')
    expect(rpShort(12_500_000)).toBe('Rp 12,5 jt')
    expect(rpShort(350_000_000)).toBe('Rp 350 jt')
    expect(rpShort(1_250_000_000)).toBe('Rp 1,25 M')
  })
  it('reads stock cover', () => {
    expect(coverText(null)).toBe('—')
    expect(coverText(0.4)).toBe('< 1 hari')
    expect(coverText(12.6)).toBe('13 hari')
  })
})
