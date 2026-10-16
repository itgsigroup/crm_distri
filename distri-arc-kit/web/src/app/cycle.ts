import type { StageName } from '../api/types'

// Live view of an Orchestrator cycle for the card, dock and pipeline. Events (POST /cycles, SSE cycle_stage /
// cycle_done, polling fallback) move a target; a 420 ms tick moves what is shown one stage at a time, so a cycle
// that finishes in milliseconds (fake LLM) still lights the six stages in turn like the mockup.

export const STAGES: StageName[] = ['ingest', 'analyze', 'synthesize', 'decide', 'execute', 'learn']
export const TICK_MS = 420

export interface CycleResult {
  number: number | null
  note: string
  updated: number
  label: string
  via: string
  status: string
}

export interface CycleUI {
  running: boolean
  cycleId: string | null
  label: string
  via: string
  mine: boolean // started from this tab → toast when it finishes
  target: number // stages reported finished (0–6); the stage in progress is `target`
  shown: number // stage shown as running (0–5); 6 = all shown done
  finished: boolean
  result: CycleResult | null
  startedAt: number | null // when this tab saw the cycle start (ms)
  // the last cycle finished in this tab, kept after its result was announced (the Pusat kendali panel shows it)
  last: (CycleResult & { mine: boolean; startedAt: number | null; endedAt: number }) | null
}

export type CycleAction =
  | { type: 'start'; cycleId: string; label: string; via: string; at?: number }
  | { type: 'stage'; cycleId: string; stage: StageName; status: string; label?: string; at?: number }
  | { type: 'done'; cycleId: string; result: CycleResult }
  | { type: 'tick' }
  | { type: 'ack'; at?: number }

export const initialCycle: CycleUI = { running: false, cycleId: null, label: '', via: 'api', mine: false, target: 0, shown: 0, finished: false, result: null, startedAt: null, last: null }

export function cycleReducer(s: CycleUI, a: CycleAction): CycleUI {
  switch (a.type) {
    case 'start':
      return { ...initialCycle, running: true, cycleId: a.cycleId, label: a.label, via: a.via, mine: true, startedAt: a.at ?? null, last: s.last }
    case 'stage': {
      const i = STAGES.indexOf(a.stage)
      if (i < 0) return s
      const base = s.running && s.cycleId === a.cycleId ? s : { ...initialCycle, running: true, cycleId: a.cycleId, label: a.label ?? 'semua', startedAt: a.at ?? null, last: s.last }
      if (s.finished && s.cycleId === a.cycleId) return s
      const target = Math.max(base.target, a.status === 'running' ? i : i + 1)
      return { ...base, target }
    }
    case 'done': {
      if (s.cycleId !== a.cycleId && s.running) return s // another cycle's late event
      const base = s.cycleId === a.cycleId ? s : { ...initialCycle, cycleId: a.cycleId, running: true, label: a.result.label, last: s.last }
      return { ...base, finished: true, target: STAGES.length, result: a.result }
    }
    case 'tick': {
      if (!s.running) return s
      if (s.shown < Math.min(s.target, STAGES.length)) return { ...s, shown: s.shown + 1 }
      if (s.finished && s.shown >= STAGES.length) return { ...s, running: false }
      return s
    }
    case 'ack':
      return { ...s, result: null, mine: false, last: s.result ? { ...s.result, mine: s.mine, startedAt: s.startedAt, endedAt: a.at ?? 0 } : s.last }
  }
}

/** Chip state of stage i: done | run | '' (mockup pipeHtml). */
export function chipState(s: CycleUI, i: number, hasCycle: boolean): 'done' | 'run' | '' {
  if (!s.running) return hasCycle ? 'done' : ''
  if (i < s.shown) return 'done'
  if (i === s.shown) return s.shown < STAGES.length ? 'run' : 'done'
  return ''
}

/** Width of the dock progress bar. */
export const progress = (s: CycleUI) => (s.running ? Math.round((Math.min(s.shown, STAGES.length - 1) + 1) / STAGES.length * 100) : 0)
