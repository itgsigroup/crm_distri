import { expect, test } from 'vitest'
import type { BoardItem } from '../../api/types'
import type { ProgressAgent } from '../control/progress'
import { boardFacts, CREW, crewLines, crewStatus, waitingBy } from './station'

const d = (name: string, m: Partial<BoardItem['metrics']>): BoardItem =>
  ({ id: name, name: 'Toko ' + name, short_name: name, metrics: { status: 'Aktif', segment: 'B', rhythm_days: 14, due_in: 30, omzet_bln: 10e6, credit: { state: 'aman' }, ...m } }) as unknown as BoardItem
const credit = (state: string) => ({ state }) as BoardItem['metrics']['credit']

const board = [
  d('Besar', { omzet_bln: 500e6 }),
  d('Jadwal', { due_in: 3 }),
  d('Tagih', { credit: credit('overdue') }),
  d('Risiko', { status: 'At risk', due_in: -9 }),
  d('Tipis', { credit: credit('tipis') }),
  d('Tutup', { status: 'Churn' }),
  d('Prospek', { status: 'Prospek', segment: 'Prospek' }),
]

test('ten astronauts in order, the last one puts the data together', () => {
  expect(CREW.map((c) => c.n)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
  expect(CREW.map((c) => c.title)).toEqual(['Analisa sales', 'Analisa WhatsApp', 'Analisa dealer', 'Eksekusi order', 'Follow-up', 'Cek stok', 'Cek kredit', 'Cek penagihan', 'Prospek', 'Data siap'])
  // every agent of the Orchestrator has a room
  expect(CREW.filter((c) => c.agent).map((c) => c.agent).sort()).toEqual(['AI Follow-up', 'AI Kredit', 'AI Order', 'AI Penagihan', 'AI Prospek', 'AI Stok'])
})

test('facts are counts from the board, never prospects', () => {
  expect(boardFacts(board)).toEqual({ dealers: 6, atRisk: 2, due: 1, tight: 1, collect: 1 })
})

test('no customer name reaches the lines', () => {
  const lines = crewLines({ ...boardFacts(board), unread: 4, newNumbers: 2, aging: 3, critical: 1, waiting: 5, waitingBy: waitingBy(['AI Order', 'AI Order', 'AI Kredit', 'AI Lain']) })
  const all = Object.values(lines).join(' ')
  for (const x of board) expect(all).not.toContain(x.name)
  expect(lines.order).toBe('1 jadwal order ≤ 14 hari · 2 usulan')
  expect(lines.credit).toBe('1 dealer limit tipis · 1 usulan')
  expect(lines.ready).toBe('5 usulan menunggu keputusan')
  expect(lines.prospect).toBe('2 nomor baru masuk')
})

const lines = crewLines({})
const agents = (state: ProgressAgent['state']): ProgressAgent[] => CREW.filter((c) => c.agent).map((c) => ({ agent: c.agent!, state, summary: state === 'done' ? '2 saran' : undefined, covers: state === 'reading' ? '12 dealer' : undefined }))

test('outside a cycle every astronaut works its own room', () => {
  const s = crewStatus(lines, null)
  expect(Object.values(s).every((w) => w.state === 'idle')).toBe(true)
  expect(s.stock.text).toBe(lines.stock)
})

test('in a cycle the rooms follow the stages, then Data siap', () => {
  const ingest = crewStatus(lines, { active: true, stage: 0, ended: false, failed: false, agents: agents('waiting'), fresh: 0 })
  expect([ingest.sales.state, ingest.wa.state, ingest.dealer.state, ingest.order.state, ingest.ready.state]).toEqual(['reading', 'reading', 'waiting', 'waiting', 'waiting'])
  const analyze = crewStatus(lines, { active: true, stage: 1, ended: false, failed: false, agents: agents('reading'), fresh: 0 })
  expect([analyze.sales.state, analyze.dealer.state, analyze.credit.state]).toEqual(['done', 'reading', 'reading'])
  expect(analyze.credit.text).toBe('membaca 12 dealer')
  const done = crewStatus(lines, { active: true, stage: 6, ended: true, failed: false, agents: agents('done'), fresh: 3 })
  expect(Object.values(done).every((w) => w.state === 'done')).toBe(true)
  expect(done.collect.text).toBe('2 saran')
  expect(done.ready.text).toBe('3 usulan baru siap')
})

test('a failed cycle leaves no astronaut reading', () => {
  const s = crewStatus(lines, { active: true, stage: 2, ended: true, failed: true, agents: agents('reading'), fresh: 0 })
  expect(Object.values(s).some((w) => w.state === 'reading' || w.state === 'waiting')).toBe(false)
  expect(s.ready.state).toBe('failed')
})
