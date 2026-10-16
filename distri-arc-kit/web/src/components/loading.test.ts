import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadingText, useDelayedFlag } from './loading'

describe('loading popup', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('waits before showing and stays a moment', () => {
    const { result, rerender } = renderHook(({ on }) => useDelayedFlag(on, 300, 500), { initialProps: { on: true } })
    expect(result.current).toBe(false)
    act(() => { vi.advanceTimersByTime(300) })
    expect(result.current).toBe(true)
    rerender({ on: false })
    act(() => { vi.advanceTimersByTime(200) })
    expect(result.current).toBe(true) // shown 200 ms of the minimum 500
    act(() => { vi.advanceTimersByTime(300) })
    expect(result.current).toBe(false)
  })

  it('never shows for a fast load', () => {
    const { result, rerender } = renderHook(({ on }) => useDelayedFlag(on, 300, 500), { initialProps: { on: true } })
    act(() => { vi.advanceTimersByTime(100) })
    rerender({ on: false })
    act(() => { vi.advanceTimersByTime(1000) })
    expect(result.current).toBe(false)
  })

  it('names what loads', () => {
    expect(loadingText([['dealers', ''], ['chat', 'threads'], ['dealer', 'x']])).toBe('Memuat dealer, chat…')
    expect(loadingText([['me'], ['kpi'], ['stock'], ['agenda'], ['x']])).toBe('Memuat akun, KPI, stok +2…')
    expect(loadingText([])).toBe('Memuat data…')
  })
})
