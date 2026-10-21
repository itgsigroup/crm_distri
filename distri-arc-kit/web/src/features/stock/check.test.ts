import { expect, test } from 'vitest'
import { diffText, parseQty, productCompare, productText, verdict, type StockProduct } from './check'

test('verdict says which side differs', () => {
  expect(verdict({ arc_qty: 43, source_qty: 43 }, 43, true)).toBe('cocok')
  expect(verdict({ arc_qty: 43, source_qty: 43 }, null, true)).toBe('belum')
  // GSI Orbit reads 24 for Jakarta, but no source row maps to Jakarta: the sync or the warehouse mapping
  expect(verdict({ arc_qty: 24, source_qty: 0 }, 0, true)).toBe('sinkron')
  // BigQuery and GSI Orbit agree, the warehouse counts other: the source data
  expect(verdict({ arc_qty: 40, source_qty: 40 }, 64, true)).toBe('sumber')
  // CSV / sample data without source rows: only GSI Orbit against the real count
  expect(verdict({ arc_qty: 5, source_qty: 0 }, 5, false)).toBe('cocok')
})

test('parseQty and diffText', () => {
  expect(parseQty('')).toBeNull()
  expect(parseQty(' 1.234 ')).toBe(1234)
  expect(parseQty('-3')).toBeNull()
  expect(parseQty('abc')).toBeNull()
  expect(parseQty('0')).toBe(0)
  expect(diffText(40, 64)).toBe('−24')
  expect(diffText(43, 43)).toBe('0')
  expect(diffText(24, 0)).toBe('+24')
})

test('stock table search and sort', () => {
  const p = (sku: string, name: string, by: Record<string, number>, at?: string): StockProduct =>
    ({ sku, name, category: 'Kamera & NVR', price: 0, qty: Object.values(by).reduce((a, b) => a + b, 0), by_branch: by, last_check: at ? { at, branches: 1, off: 0 } : null })
  const a = p('X5S_8W', 'Ezviz NVR 8Ch X5S 8W', { Surabaya: 43, Jakarta: 24 }, '2026-10-21T03:00:00Z')
  const b = p('H8C', 'Ezviz H8c 2MP', { Surabaya: 5 })
  const c = productCompare(['Jakarta', 'Surabaya'])
  expect([b, a].sort(c.total).map((x) => x.sku)).toEqual(['H8C', 'X5S_8W'])
  expect(c['b:Jakarta'](b, a)).toBeLessThan(0)
  expect(c.cek(b, a)).toBeLessThan(0)
  expect(productText(a).toLowerCase()).toContain('x5s_8w')
})
