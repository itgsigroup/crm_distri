import { describe, expect, it } from 'vitest'
import type { LyraLine } from '../../api/types'
import { discounted, groupRupiah, parsePct, parseRupiah, quoteText, rupiah, startQty, totals } from './lyra'

const nvr: LyraLine = { sku: 'X5S_8W', product: 'Ezviz NVR 8Ch X5S 8W', asked: 2, price: 865000, stock: 42, cities: [{ city: 'Surabaya', qty: 30 }, { city: 'Semarang', qty: 12 }] }
const cam: LyraLine = { sku: 'H8C', product: 'Ezviz H8C 2MP', asked: 0, price: 0, stock: 0, cities: [] }

describe('Lyra card maths (ADR 0070)', () => {
  it('reads a typed discount as a percentage 0–100', () => {
    expect(parsePct('5')).toBe(5)
    expect(parsePct('2,5')).toBe(2.5)
    expect(parsePct('12.5%')).toBe(12.5)
    expect(parsePct('')).toBe(0)
    expect(parsePct('-3')).toBe(3) // the minus sign is not a negative discount
    expect(parsePct('150')).toBe(100)
  })
  it('discounts the unit price and the totals', () => {
    expect(discounted(865000, 5)).toBe(821750)
    expect(discounted(865000, 2.5)).toBe(843375)
    const t = totals([{ line: nvr, qty: 2 }, { line: cam, qty: 1 }], 5)
    expect(t).toEqual({ lines: [1643500, 0], total: 1643500, before: 1730000 })
    expect(startQty(nvr)).toBe(2)
    expect(startQty(cam)).toBe(1)
    expect(rupiah(1643500)).toBe('Rp 1.643.500')
  })
  it('writes the message with stock per city, the discounted DPP price and the total', () => {
    const t = quoteText([{ line: nvr, qty: 2 }, { line: cam, qty: 1 }], 5)
    expect(t).toContain('*1. Ezviz NVR 8Ch X5S 8W*')
    expect(t).toContain('Stok: Surabaya 30 · Semarang 12')
    expect(t).toContain('Harga: ~Rp 865.000~ diskon 5% → *Rp 821.750*/unit')
    expect(t).toContain('2 unit × Rp 821.750 = *Rp 1.643.500*')
    expect(t).toContain('Stok: kosong (bisa inden)')
    expect(t).toContain('Harga: kami konfirmasi segera')
    expect(t).toContain('*Total: Rp 1.643.500*')
    expect(t).toContain('Harga DPP, belum termasuk PPN.')
    const one = quoteText([{ line: nvr, qty: 1 }], 0)
    expect(one).toContain('*Ezviz NVR 8Ch X5S 8W*')
    expect(one).toContain('Harga: *Rp 865.000*/unit')
    expect(one).not.toContain('Total')
  })
})

describe('edit harga (the sales sets the unit price)', () => {
  it('reads a typed price in whole rupiah and groups it back', () => {
    expect(parseRupiah('711.712')).toBe(711712)
    expect(parseRupiah('Rp 650.000')).toBe(650000)
    expect(parseRupiah('')).toBe(0)
    expect(groupRupiah(650000)).toBe('650.000')
    expect(groupRupiah(0)).toBe('')
  })
  it('quotes the edited price, with the discount on it', () => {
    const text = quoteText([{ line: { ...nvr, price: 800000 }, qty: 2 }], 10)
    expect(text).toContain('~Rp 800.000~ diskon 10% → *Rp 720.000*/unit')
    expect(text).toContain('2 unit × Rp 720.000 = *Rp 1.440.000*')
  })
})
