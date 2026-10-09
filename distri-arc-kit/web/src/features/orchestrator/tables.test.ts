import { expect, test } from 'vitest'
import type { Conflict, Cycle } from '../../api/types'
import { CONFLICT_COMPARE, RUN_COMPARE, conflictText, finishedRuns, runFirstDir, runText, type RunColumn } from './tables'

const cyc = (number: number, o: Partial<Cycle>): Cycle =>
  ({ id: 'c' + number, number, trigger: 'schedule', scope: 'all', via: 'api', requested_by: null, status: 'done', started_at: '2026-10-05T01:00:00Z', finished_at: null, duration_ms: 1, signals_count: 0, auto_count: 0, decision_count: 0, conflict_count: 0, note: '', stage: null, stages: [], label: '', ...o }) as Cycle
const runs = [
  cyc(3, { started_at: '2026-10-05T03:00:00Z', signals_count: 40, via: 'mcp', note: 'lewat Claude Desktop' }),
  cyc(1, { started_at: '2026-10-05T01:00:00Z', signals_count: 90, decision_count: 4 }),
  cyc(2, { started_at: '2026-10-05T02:00:00Z', conflict_count: 3, note: 'agen kredit gagal' }),
  cyc(4, { status: 'running' }),
]
const sort = (c: RunColumn, dir: 'asc' | 'desc') => [...finishedRuns(runs)].sort((a, b) => RUN_COMPARE[c](a, b) * (dir === 'asc' ? 1 : -1)).map((r) => r.number)

test('history: only finished cycles; every column sorts both ways', () => {
  expect(finishedRuns(runs).map((r) => r.number)).toEqual([3, 1, 2])
  expect(sort('jam', 'desc')).toEqual([3, 2, 1])
  expect(sort('jam', 'asc')).toEqual([1, 2, 3])
  expect(sort('sinyal', 'desc')[0]).toBe(1)
  expect(sort('konflik', 'desc')[0]).toBe(2)
  expect(sort('keputusan', 'asc')[2]).toBe(1)
  expect(sort('jalur', 'desc')[0]).toBe(3)
  expect(runFirstDir('jam')).toBe('desc')
  expect(runFirstDir('catatan')).toBe('asc')
  expect(runText(runs[0])).toContain('mcp')
})

test('conflicts: search text and sorting by agent, dealer, conflict, resolution', () => {
  const c = (id: string, o: Partial<Conflict>): Conflict => ({ id, cycle_id: 'x', dealer_id: null, agent_a: 'AI Order', agent_b: 'AI Kredit', title: '', resolution: '', rule: '', tone: null, visible: true, dealer_name: null, dealer_slug: null, ...o })
  const list = [c('1', { agent_a: 'AI Stok', dealer_name: 'Sinar', title: 'Stok vs limit' }), c('2', { dealer_name: 'Graha', title: 'Rilis SO' })]
  expect(conflictText(list[0])).toContain('Sinar')
  expect([...list].sort(CONFLICT_COMPARE.dealer).map((x) => x.id)).toEqual(['2', '1'])
  expect([...list].sort(CONFLICT_COMPARE.agen).map((x) => x.id)).toEqual(['2', '1'])
  expect([...list].sort(CONFLICT_COMPARE.konflik).map((x) => x.id)).toEqual(['2', '1'])
})
