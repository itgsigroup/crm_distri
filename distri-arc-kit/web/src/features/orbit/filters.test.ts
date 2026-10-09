import { expect, test } from 'vitest'
import type { BoardItem } from '../../api/types'
import { EMPTY, activeCount, applyFilter, digest, sortDealers } from './filters'

type M = BoardItem['metrics']
const dealer = (name: string, m: Partial<M>, over: Partial<BoardItem> = {}): BoardItem =>
  ({
    id: name, name, short_name: name, city: 'Semarang', branch: 'Semarang', owner: { name: 'Andi' },
    metrics: { status: 'Aktif', rhythm_days: 14, due_in: 5, last_order_days: 9, omzet_bln: 10e6, segment: 'B', credit: { state: 'aman', exposure: 0 }, ...m },
    ...over,
  }) as unknown as BoardItem

const list = [
  dealer('Sinar', { status: 'Key account', due_in: 2, omzet_bln: 130e6, segment: 'A' }),
  dealer('Mitra', { status: 'At risk', due_in: -9, last_order_days: 30, credit: { state: 'over limit', exposure: 90e6 } as M['credit'], segment: 'C' }),
  dealer('Lampu', { status: 'Churn', due_in: -40, last_order_days: 75, omzet_bln: 3e6 }, { branch: 'Solo', city: 'Solo' }),
  dealer('Baru', { status: 'Baru', rhythm_days: null, due_in: null, last_order_days: 9, credit: { state: 'cash', exposure: 0 } as M['credit'], segment: 'Baru' }),
]

test('jadwal order 2 minggu: 0 ≤ due_in ≤ 14 with a cycle, never Churn or Baru', () => {
  expect(applyFilter(list, { ...EMPTY, jadwal: 'minggu' }).map((d) => d.id)).toEqual(['Sinar'])
  const edge = [dealer('Hari14', { due_in: 14 }), dealer('Hari15', { due_in: 15 }), dealer('Hari8', { due_in: 8 })]
  expect(applyFilter(edge, { ...EMPTY, jadwal: 'minggu' }).map((d) => d.id)).toEqual(['Hari14', 'Hari8'])
})

test('lewat jadwal is the At risk ring; tagih dulu is over limit or overdue', () => {
  expect(applyFilter(list, { ...EMPTY, jadwal: 'lewat' }).map((d) => d.id)).toEqual(['Mitra'])
  expect(applyFilter(list, { ...EMPTY, limit: 'tagih' }).map((d) => d.id)).toEqual(['Mitra'])
})

test('status Aktif includes Baru (drawn on the Aktif ring); filters combine', () => {
  expect(applyFilter(list, { ...EMPTY, status: 'Aktif' }).map((d) => d.id)).toEqual(['Baru'])
  expect(applyFilter(list, { ...EMPTY, cabang: 'Solo', q: 'lam' }).map((d) => d.id)).toEqual(['Lampu'])
  expect(applyFilter(list, { ...EMPTY, cabang: 'Solo', segmen: 'A' })).toEqual([])
  expect(activeCount({ ...EMPTY, cabang: 'Solo', q: 'x' })).toBe(2)
})

test('sort: jadwal puts the most overdue first and dealers without a cycle last', () => {
  expect(sortDealers(list, 'jadwal').map((d) => d.id)).toEqual(['Lampu', 'Mitra', 'Sinar', 'Baru'])
  expect(sortDealers(list, 'diam')[0].id).toBe('Lampu')
  expect(sortDealers(list, 'omzet')[0].id).toBe('Sinar')
})

test('digest counts the summary tiles from the same definitions', () => {
  const g = digest(list)
  expect(g).toMatchObject({ total: 4, due: { n: 1, omzet: 130e6 }, late: { n: 1 }, collect: { n: 1, exposure: 90e6 }, churn: { n: 1, omzet: 3e6 } })
})
