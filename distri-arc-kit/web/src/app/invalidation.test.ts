import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { InvalidationQueue } from './SseProvider'

describe('InvalidationQueue', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('a burst of identical events refetches once, distinct keys once each', () => {
    const run = vi.fn()
    const q = new InvalidationQueue(run, 1000, () => Date.now())
    for (let i = 0; i < 200; i++) q.add(['chat'])
    q.add(['chat', 'wa-analysis'])
    vi.advanceTimersByTime(10)
    expect(run.mock.calls.map((c) => c[0])).toEqual([['chat'], ['chat', 'wa-analysis']])
  })

  it('events keep arriving: at most one refetch per window, none lost', () => {
    const run = vi.fn()
    const q = new InvalidationQueue(run, 1000, () => Date.now())
    q.add(['chat'])
    vi.advanceTimersByTime(10) // leading flush
    expect(run).toHaveBeenCalledTimes(1)
    for (let t = 0; t < 5; t++) {
      q.add(['chat'])
      vi.advanceTimersByTime(100)
    }
    expect(run).toHaveBeenCalledTimes(1) // held back inside the window
    vi.advanceTimersByTime(1000)
    expect(run).toHaveBeenCalledTimes(2) // the trailing one: the last events are not lost
  })
})
