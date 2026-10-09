import { expect, test } from 'vitest'
import type { AgingItem } from '../../api/types'
import { AGING_COMPARE, agingText, agingTie, firstDir, type StockColumn } from './table'

const item = (name: string, o: Partial<AgingItem>): AgingItem =>
  ({ id: name, name, sku: 'SKU-' + name, category: 'CCTV', branch: 'Semarang', qty: 10, unit_cost: 1, value: 1e6, age_days: 100, weekly_velocity: 1, candidates: [], candidate_count: 0, due_this_week: 0, ...o }) as AgingItem

const list = [
  item('Kamera IP 2MP', { branch: 'Solo', qty: 40, value: 12e6, age_days: 130, candidates: [{ dealer_id: 'sinar', name: 'Toko Sinar Elektronik', reason: '', due_in: 3, drifting: false, omzet_bln: 0 }], candidate_count: 1 }),
  item('DVR 8 channel', { qty: 5, value: 30e6, age_days: 95 }),
  item('Kabel coaxial', { branch: 'Bekasi', qty: 300, value: 4e6, age_days: 210, candidate_count: 7 }),
]
const find = (q: string) => list.filter((x) => agingText(x).toLocaleLowerCase('id-ID').includes(q.toLocaleLowerCase('id-ID'))).map((x) => x.name)
const sort = (c: StockColumn, dir: 'asc' | 'desc') => [...list].sort((a, b) => (AGING_COMPARE[c](a, b) || agingTie(a, b)) * (dir === 'asc' ? 1 : -1))

test('search text covers product, SKU, branch and matching dealer names', () => {
  expect(find('kamera')).toEqual(['Kamera IP 2MP'])
  expect(find('sku-dvr')).toEqual(['DVR 8 channel'])
  expect(find('bekasi')).toEqual(['Kabel coaxial'])
  expect(find('SINAR')).toEqual(['Kamera IP 2MP'])
})

test('every column sorts both ways', () => {
  expect(sort('umur', 'desc').map((x) => x.age_days)).toEqual([210, 130, 95])
  expect(sort('umur', 'asc').map((x) => x.age_days)).toEqual([95, 130, 210])
  expect(sort('nilai', 'desc')[0].name).toBe('DVR 8 channel')
  expect(sort('qty', 'asc')[0].name).toBe('DVR 8 channel')
  expect(sort('produk', 'asc').map((x) => x.name)).toEqual(['DVR 8 channel', 'Kabel coaxial', 'Kamera IP 2MP'])
  expect(sort('cabang', 'desc')[0].branch).toBe('Solo')
  expect(sort('dealer', 'desc')[0].name).toBe('Kabel coaxial')
  expect(firstDir('produk')).toBe('asc')
  expect(firstDir('nilai')).toBe('desc')
})
