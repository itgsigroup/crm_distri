import { expect, test } from 'vitest'
import type { BoardItem, ThreadView } from '../../api/types'
import { capsules, orbitOf, planStation } from './station'

const d = (name: string, m: Partial<BoardItem['metrics']>): BoardItem =>
  ({ id: name, name: 'Toko ' + name, short_name: name, metrics: { status: 'Aktif', segment: 'B', rhythm_days: 14, due_in: 30, omzet_bln: 10e6, credit: { state: 'aman' }, ...m } }) as unknown as BoardItem
const credit = (state: string) => ({ state }) as BoardItem['metrics']['credit']

const board = [
  ...Array.from({ length: 20 }, (_, i) => d('Besar' + i, { omzet_bln: 500e6 - i * 1e6 })),
  d('Jadwal', { due_in: 3, omzet_bln: 1e6 }),
  d('Tagih', { credit: credit('overdue'), omzet_bln: 1e6 }),
  d('Risiko', { status: 'At risk', due_in: -9, omzet_bln: 1e6 }),
  d('Tipis', { credit: credit('tipis'), omzet_bln: 1e6 }),
  d('Tutup', { status: 'Churn', omzet_bln: 2e6 }),
  d('Prospek', { status: 'Prospek', segment: 'Prospek', omzet_bln: 900e6 }),
]
const th = (id: string, m: Partial<ThreadView>): ThreadView => ({ id, kind: 'new', title: id, last_message_at: '2026-10-20T01:00:00Z', ...m }) as ThreadView

test('planets: dealers with a task even when small, then the biggest; never prospects', () => {
  const p = planStation(board, [], 10)
  const ids = p.planets.map((s) => s.id)
  expect(ids).toHaveLength(10)
  expect(ids).toEqual(expect.arrayContaining(['Jadwal', 'Tagih', 'Risiko', 'Tipis', 'Besar0']))
  expect(ids).not.toContain('Prospek')
})

test('astronaut tasks come from the data', () => {
  const p = planStation(board, [], 10)
  expect(p.tasks.order).toEqual(['Jadwal'])
  expect(p.tasks.collect).toEqual(['Tagih'])
  expect(p.tasks.credit).toEqual(['Tipis'])
  expect(p.tasks.followup).toEqual(['Risiko'])
  expect(p.planets.find((s) => s.id === 'Risiko')!.look).toBe('risk')
  expect(p.planets.find((s) => s.id === 'Tagih')!.tone).toBe('bad')
})

test('orbit: the nearer the jadwal order, the closer to the core; late and churn at the ends', () => {
  expect(orbitOf(-9)).toBe(0)
  expect(orbitOf(0)).toBe(0)
  expect(orbitOf(3)).toBeLessThan(orbitOf(30))
  expect(orbitOf(400)).toBeCloseTo(0.92)
  expect(orbitOf(null)).toBe(1)
  expect(orbitOf(3, true)).toBe(1)
})

test('size follows omzet; the biggest is 1', () => {
  const p = planStation(board, [], 10)
  expect(p.planets[0].id).toBe('Besar0')
  expect(p.planets[0].size).toBe(1)
  for (let i = 1; i < p.planets.length; i++) expect(p.planets[i].size).toBeLessThanOrEqual(p.planets[i - 1].size)
  expect(planStation([], [], 8).planets).toEqual([])
})

test('comets are inbound new numbers only: no suppliers, no numbers GSI wrote to first; newest first', () => {
  const p = planStation(board, [
    th('a', { last_message_at: '2026-10-20T01:00:00Z' }),
    th('b', { last_message_at: '2026-10-20T03:00:00Z' }),
    th('sup', { contact_type: 'supplier' }),
    th('out', { last_direction: 'out' }),
    th('dealer', { kind: 'dealer' }),
  ], 6)
  expect(p.comets.map((c) => c.id)).toEqual(['b', 'a'])
})

test('capsules at the dock: one per waiting proposal, by agent', () => {
  expect(capsules(['AI Kredit', 'AI Stok', 'lain'])).toEqual(['credit', 'stock', 'order'])
  expect(capsules(Array(30).fill('AI Order'))).toHaveLength(12)
})
