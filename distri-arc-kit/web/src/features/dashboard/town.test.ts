import { expect, test } from 'vitest'
import type { BoardItem } from '../../api/types'
import { planTown } from './town'

const d = (name: string, m: Partial<BoardItem['metrics']>): BoardItem =>
  ({ id: name, name: 'Toko ' + name, short_name: name, metrics: { status: 'Aktif', segment: 'B', rhythm_days: 14, due_in: 30, omzet_bln: 10e6, credit: { state: 'aman' }, ...m } }) as unknown as BoardItem

const board = [
  ...Array.from({ length: 20 }, (_, i) => d('Besar' + i, { omzet_bln: 500e6 - i * 1e6 })),
  d('Jadwal', { due_in: 3, omzet_bln: 1e6 }),
  d('Tagih', { credit: { state: 'overdue' } as BoardItem['metrics']['credit'], omzet_bln: 1e6 }),
  d('Risiko', { status: 'At risk', due_in: -9, omzet_bln: 1e6 }),
  d('Tutup', { status: 'Churn', omzet_bln: 2e6 }),
  d('Prospek', { status: 'Prospek', segment: 'Prospek', omzet_bln: 900e6 }),
]

test('the town shows dealers with a task even when they are small, then the biggest; never prospects', () => {
  const p = planTown(board, 8)
  const ids = p.shops.map((s) => s.id)
  expect(ids).toHaveLength(8)
  expect(ids).toEqual(expect.arrayContaining(['Jadwal', 'Tagih', 'Risiko']))
  expect(ids).not.toContain('Prospek')
  expect(ids).toContain('Besar0')
})

test('tasks come from the data: deliver = jadwal order near, collect = over limit/overdue, visit = At risk', () => {
  const p = planTown(board, 8)
  expect(p.tasks.deliver.map((t) => t.shop)).toEqual(['Jadwal'])
  expect(p.tasks.collect.map((t) => t.shop)).toEqual(['Tagih'])
  expect(p.tasks.visit.map((t) => t.shop)).toEqual(['Risiko'])
  expect(p.shops.find((s) => s.id === 'Risiko')!.look).toBe('risk')
  expect(p.shops.find((s) => s.id === 'Tagih')!.tone).toBe('bad')
})

test('shops split over two streets with slots from 0 to 1', () => {
  const p = planTown(board, 8)
  const front = p.shops.filter((s) => s.row === 0)
  expect(front).toHaveLength(4)
  expect(front.map((s) => s.slot)).toEqual([0, 1 / 3, 2 / 3, 1])
  expect(planTown([], 8).shops).toEqual([])
})
