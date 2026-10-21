import { expect, test } from 'vitest'
import { diffText, parseQty, verdict } from './check'

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
