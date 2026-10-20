import { expect, test } from 'vitest'
import type { BoardItem } from '../../api/types'
import { planMine } from './mine'

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

test('the mine shows dealers with a task even when they are small, then the biggest; never prospects', () => {
  const p = planMine(board, 8)
  const ids = p.sites.map((s) => s.id)
  expect(ids).toHaveLength(8)
  expect(ids).toEqual(expect.arrayContaining(['Jadwal', 'Tagih', 'Risiko']))
  expect(ids).not.toContain('Prospek')
  expect(ids).toContain('Besar0')
})

test('tasks come from the data: deliver = jadwal order near, collect = over limit/overdue, visit = At risk', () => {
  const p = planMine(board, 8)
  expect(p.tasks.deliver.map((t) => t.site)).toEqual(['Jadwal'])
  expect(p.tasks.collect.map((t) => t.site)).toEqual(['Tagih'])
  expect(p.tasks.visit.map((t) => t.site)).toEqual(['Risiko'])
  expect(p.sites.find((s) => s.id === 'Risiko')!.look).toBe('risk')
  expect(p.sites.find((s) => s.id === 'Tagih')!.tone).toBe('bad')
})

test('sites spread down the haul road, biggest omzet nearest the rim', () => {
  const p = planMine(board, 8)
  const ts = p.sites.map((s) => s.t)
  expect(ts[0]).toBeCloseTo(0.14)
  expect(ts[ts.length - 1]).toBeCloseTo(0.94)
  expect([...ts].sort((a, b) => a - b)).toEqual(ts)
  expect(p.sites[0].id).toBe('Besar0')
  expect(planMine([], 8).sites).toEqual([])
  expect(planMine(board.slice(0, 1), 8).sites[0].t).toBeCloseTo(0.54)
})
