import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { POLL_MS, SILENT_MS, SseProvider } from './SseProvider'

// A stand-in EventSource: the test plays the server.
class FakeES {
  static all: FakeES[] = []
  static CLOSED = 2
  readyState = 1
  onerror: (() => void) | null = null
  closed = false
  private ls = new Map<string, ((e: MessageEvent) => void)[]>()
  url: string
  constructor(url: string) { this.url = url; FakeES.all.push(this) }
  addEventListener(n: string, f: (e: MessageEvent) => void) { this.ls.set(n, [...(this.ls.get(n) ?? []), f]) }
  removeEventListener() {}
  close() { this.closed = true; this.readyState = 2 }
  emit(n: string, data = '{}') { (this.ls.get(n) ?? []).forEach((f) => f({ data } as MessageEvent)) }
}

describe('realtime stream', () => {
  let qc: QueryClient
  let invalidated: string[]
  beforeEach(() => {
    vi.useFakeTimers()
    FakeES.all = []
    vi.stubGlobal('EventSource', FakeES)
    qc = new QueryClient()
    invalidated = []
    vi.spyOn(qc, 'invalidateQueries').mockImplementation(async (f?: { queryKey?: readonly unknown[] }) => { invalidated.push(JSON.stringify(f?.queryKey)) })
  })
  afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })

  it('reconnects a silent stream and catches up on what it missed', () => {
    render(<QueryClientProvider client={qc}><SseProvider><div /></SseProvider></QueryClientProvider>)
    const first = FakeES.all[0]
    act(() => { first.emit('hello') })
    expect(invalidated).toEqual([]) // the first connection: the page just loaded its data
    act(() => { vi.advanceTimersByTime(SILENT_MS + 6000) }) // no heartbeat: dead
    expect(first.closed).toBe(true)
    const second = FakeES.all[1]
    expect(second).toBeTruthy()
    act(() => { second.emit('hello'); vi.advanceTimersByTime(2000) })
    expect(invalidated).toContain(JSON.stringify(['chat', 'threads'])) // caught up
    expect(invalidated).toContain(JSON.stringify(['chat', 'thread']))
  })

  it('stays connected while heartbeats come, and polls the chats while down', () => {
    render(<QueryClientProvider client={qc}><SseProvider><div /></SseProvider></QueryClientProvider>)
    const es = FakeES.all[0]
    act(() => { es.emit('hello') })
    for (let i = 0; i < 6; i++) act(() => { vi.advanceTimersByTime(25_000); es.emit('heartbeat') })
    expect(FakeES.all.length).toBe(1) // never reconnected
    act(() => { es.onerror?.() }) // down: the browser retries; meanwhile the chats poll
    act(() => { vi.advanceTimersByTime(POLL_MS + 5000) })
    expect(invalidated).toContain(JSON.stringify(['chat', 'threads']))
  })
})
