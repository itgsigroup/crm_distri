import type { Conflict, Cycle } from '../../api/types'
import type { Dir } from '../../components/DataTable'

// Orchestrator tables (Resolusi konflik, Riwayat analisis): searchable text and per-column compares (tested).

export type ConflictColumn = 'agen' | 'dealer' | 'konflik' | 'resolusi'
export type RunColumn = 'jam' | 'no' | 'jalur' | 'sinyal' | 'otonom' | 'keputusan' | 'konflik' | 'catatan'

const txt = (a: string | null | undefined, b: string | null | undefined) => (a ?? '').localeCompare(b ?? '', 'id')

export const conflictText = (c: Conflict) => `${c.agent_a} ${c.agent_b} ${c.dealer_name ?? ''} ${c.title} ${c.resolution} ${c.rule}`
export const CONFLICT_COMPARE: Record<ConflictColumn, (a: Conflict, b: Conflict) => number> = {
  agen: (a, b) => txt(a.agent_a, b.agent_a) || txt(a.agent_b, b.agent_b),
  dealer: (a, b) => txt(a.dealer_name, b.dealer_name),
  konflik: (a, b) => txt(a.title, b.title),
  resolusi: (a, b) => txt(a.resolution, b.resolution),
}
export const conflictTie = (a: Conflict, b: Conflict) => txt(a.id, b.id)
export const conflictFirstDir = (): Dir => 'asc'

/** Finished cycles only: queued / running ones are shown by the pipeline, not the history. */
export const finishedRuns = (cycles: Cycle[]) => cycles.filter((c) => c.status !== 'queued' && c.status !== 'running')

export const runText = (r: Cycle) => `#${r.number ?? ''} ${r.via ?? 'api'} ${r.trigger} ${r.scope} ${r.status} ${r.note ?? ''}`
const n = (v: number | null | undefined) => v ?? 0
export const RUN_COMPARE: Record<RunColumn, (a: Cycle, b: Cycle) => number> = {
  jam: (a, b) => Date.parse(a.started_at) - Date.parse(b.started_at),
  no: (a, b) => n(a.number) - n(b.number),
  jalur: (a, b) => txt(a.via ?? 'api', b.via ?? 'api'),
  sinyal: (a, b) => n(a.signals_count) - n(b.signals_count),
  otonom: (a, b) => n(a.auto_count) - n(b.auto_count),
  keputusan: (a, b) => n(a.decision_count) - n(b.decision_count),
  konflik: (a, b) => n(a.conflict_count) - n(b.conflict_count),
  catatan: (a, b) => txt(a.note, b.note),
}
export const runTie = (a: Cycle, b: Cycle) => n(a.number) - n(b.number)
/** Text columns start A→Z, numbers and time start with the largest / newest. */
export const runFirstDir = (c: RunColumn): Dir => (c === 'jalur' || c === 'catatan' ? 'asc' : 'desc')
