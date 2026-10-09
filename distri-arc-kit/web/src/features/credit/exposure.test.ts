import { expect, test } from 'vitest'
import type { BoardItem, ExposureRow } from '../../api/types'
import { EXPOSURE_COMPARE, exposureLines, exposureText, exposureTie, type ExposureColumn } from './exposure'

const ex: ExposureRow[] = [
  { dealer_id: 'mitra', short_name: 'Mitra Jaya Teknik', exposure: 162e6, limit: 150e6, pct: 108 },
  { dealer_id: 'nusa', short_name: 'Nusa Teknik', exposure: 98e6, limit: 100e6, pct: 98 },
  { dealer_id: 'sinar', short_name: 'Sinar Elektronik', exposure: 176e6, limit: 250e6, pct: 70 },
]
const board = [{ id: 'sinar', name: 'Toko Sinar Elektronik', branch: 'Semarang', owner: { name: 'Andi' } }] as unknown as BoardItem[]
const rows = exposureLines(ex, board)
const sort = (c: ExposureColumn, dir: 'asc' | 'desc') => [...rows].sort((a, b) => (EXPOSURE_COMPARE[c](a, b) || exposureTie(a, b)) * (dir === 'asc' ? 1 : -1)).map((x) => x.dealer_id)

test('rows get the dealer name, sales, cabang and remaining limit (negative when over)', () => {
  const sinar = rows.find((x) => x.dealer_id === 'sinar')!
  expect(sinar).toMatchObject({ name: 'Toko Sinar Elektronik', owner: 'Andi', branch: 'Semarang', room: 74e6 })
  expect(rows.find((x) => x.dealer_id === 'mitra')!.room).toBe(-12e6)
  expect(exposureText(sinar)).toContain('Andi')
})

test('every column sorts both ways', () => {
  expect(sort('pct', 'desc')).toEqual(['mitra', 'nusa', 'sinar'])
  expect(sort('pct', 'asc')).toEqual(['sinar', 'nusa', 'mitra'])
  expect(sort('room', 'asc')[0]).toBe('mitra')
  expect(sort('exposure', 'desc')[0]).toBe('sinar')
  expect(sort('limit', 'asc')[0]).toBe('nusa')
  expect(sort('dealer', 'asc')).toEqual(['mitra', 'nusa', 'sinar'])
})
