import { expect, test } from 'vitest'
import type { BoardItem, CreditState, Segment, Status } from '../../api/types'
import { applySegmenFilters, EMPTY_SEGMEN_FILTER, summarizeSegmen } from './filters'

const dealer = (id: number, omzet: number, segment: Segment, branch = 'Jakarta', status: Status = 'Aktif', credit: CreditState = 'aman', prev: Segment | null = segment): BoardItem => ({
  id: String(id), name: `Dealer ${id}`, short_name: `D${id}`, city: id === 4 ? 'Bandung' : 'Jakarta', branch,
  owner: { key: 'sales', name: 'Sales Satu', initials: 'SS', branch },
  metrics: { omzet_bln: omzet, segment, status, credit: { state: credit } },
  prev: prev === null ? null : { segment: prev },
}) as unknown as BoardItem

const list = [
  dealer(1, 80, 'A', 'Jakarta', 'Aktif', 'tipis', 'B'),
  dealer(2, 70, 'A'),
  dealer(3, 60, 'B'),
  dealer(4, 50, 'B', 'Bandung'),
  dealer(5, 40, 'C'),
  dealer(6, 30, 'C'),
  dealer(7, 20, 'D'),
  dealer(8, 10, 'Baru', 'Jakarta', 'Baru', 'cash', null),
]

test('segment filters combine dealer search, branch, status, credit, and movement', () => {
  const result = applySegmenFilters(list, { ...EMPTY_SEGMEN_FILTER, q: 'sales satu', cabang: 'Jakarta', status: 'Aktif', credit: 'tipis', change: 'moved' })
  expect(result.map((d) => d.id)).toEqual(['1'])
  expect(applySegmenFilters(list, { ...EMPTY_SEGMEN_FILTER, q: 'Bandung' }).map((d) => d.id)).toEqual(['4'])
})

test('revenue cohorts rank dealers within the filtered sales data', () => {
  expect(applySegmenFilters(list, { ...EMPTY_SEGMEN_FILTER, revenue: 'top10' }).map((d) => d.id)).toEqual(['1'])
  expect(applySegmenFilters(list, { ...EMPTY_SEGMEN_FILTER, revenue: 'top25' }).map((d) => d.id)).toEqual(['1', '2'])
  expect(applySegmenFilters(list, { ...EMPTY_SEGMEN_FILTER, revenue: 'bottom25' }).map((d) => d.id)).toEqual(['7', '8'])
  expect(applySegmenFilters(list, { ...EMPTY_SEGMEN_FILTER, cabang: 'Bandung', revenue: 'top25' }).map((d) => d.id)).toEqual(['4'])
})

test('box counts and revenue shares use the filtered cohort', () => {
  const filtered = list.filter((d) => d.id === '1' || d.id === '7')
  const summary = summarizeSegmen(filtered)
  expect(summary.total).toBe(100)
  expect(summary.items.find((row) => row.segment === 'A')).toMatchObject({ count: 1, omzet_bln: 80, pct: 80 })
  expect(summary.items.find((row) => row.segment === 'D')).toMatchObject({ count: 1, omzet_bln: 20, pct: 20 })
  expect(summary.items.find((row) => row.segment === 'B')).toMatchObject({ count: 0, omzet_bln: 0, pct: 0 })
})
