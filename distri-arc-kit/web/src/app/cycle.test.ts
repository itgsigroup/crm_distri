import { describe, expect, it } from 'vitest'
import { STAGES, chipState, cycleReducer, initialCycle, progress, type CycleAction, type CycleUI } from './cycle'

const run = (actions: CycleAction[], s: CycleUI = initialCycle) => actions.reduce(cycleReducer, s)
const result = { number: 2, note: 'Analisis ulang orbit (API) · 1 saran baru', updated: 1, label: 'orbit', via: 'api', status: 'done' }

describe('useCycle reducer', () => {
  it('lights the stages one tick at a time even when the cycle finishes at once', () => {
    let s = run([{ type: 'start', cycleId: 'c1', label: 'orbit', via: 'api' }])
    expect(s.running && s.mine).toBe(true)
    expect(chipState(s, 0, true)).toBe('run')
    s = run([...STAGES.map((st) => ({ type: 'stage', cycleId: 'c1', stage: st, status: 'done' }) as CycleAction), { type: 'done', cycleId: 'c1', result }], s)
    expect(s.shown).toBe(0) // nothing jumps ahead before the tick
    s = run([{ type: 'tick' }, { type: 'tick' }], s)
    expect(chipState(s, 0, true)).toBe('done')
    expect(chipState(s, 1, true)).toBe('done')
    expect(chipState(s, 2, true)).toBe('run')
    expect(chipState(s, 3, true)).toBe('')
    expect(progress(s)).toBe(50)
    s = run(Array.from({ length: 4 }, () => ({ type: 'tick' }) as CycleAction), s)
    expect(s.running).toBe(true) // all six shown done, one more tick to settle
    s = cycleReducer(s, { type: 'tick' })
    expect(s.running).toBe(false)
    expect(s.result?.updated).toBe(1)
    expect(chipState(s, 5, true)).toBe('done')
    s = cycleReducer(s, { type: 'ack' })
    expect(s.result).toBeNull()
  })

  it('never shows a stage the server has not reached', () => {
    let s = run([{ type: 'start', cycleId: 'c1', label: 'semua', via: 'api' }, { type: 'stage', cycleId: 'c1', stage: 'analyze', status: 'running' }])
    s = run([{ type: 'tick' }, { type: 'tick' }, { type: 'tick' }], s)
    expect(s.shown).toBe(1)
    expect(chipState(s, 1, true)).toBe('run')
  })

  it('follows a scheduled cycle started elsewhere without claiming it', () => {
    const s = run([{ type: 'stage', cycleId: 'sched', stage: 'ingest', status: 'running', label: 'semua' }])
    expect(s.running).toBe(true)
    expect(s.mine).toBe(false)
  })

  it('ignores late events of another cycle', () => {
    const s = run([{ type: 'start', cycleId: 'c2', label: 'orbit', via: 'api' }, { type: 'done', cycleId: 'c1', result }])
    expect(s.finished).toBe(false)
  })

  it('idle: chips done after a cycle, neutral before the first', () => {
    expect(chipState(initialCycle, 3, true)).toBe('done')
    expect(chipState(initialCycle, 3, false)).toBe('')
  })
})
