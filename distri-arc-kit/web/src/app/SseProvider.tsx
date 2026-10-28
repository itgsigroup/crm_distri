import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

// One EventSource('/api/events') per tab; each event invalidates the queries it affects (08-frontend).
export const CHAT_LIVE = [['chat', 'threads'], ['chat', 'thread'], ['chat', 'context']]
const INVALIDATES: Record<string, string[][]> = {
  cycle_stage: [['cycle']],
  cycle_done: [['cycle'], ['cycles'], ['plan'], ['proposals'], ['brief'], ['orbit'], ['segmen'], ['dealers'], ['dealer'], ['kpi'], ['agenda'], ['agents'], ['stock'], ['credit'], ['chat', 'wa-analysis']],
  proposal_changed: [['proposals'], ['plan'], ['dealer'], ['dealers'], ['cycle'], ['orbit'], ...CHAT_LIVE],
  // only what a new message changes: not Analisa WhatsApp (the pill on every page refetched it on each message,
  // ~11,700×/day), labels or group members
  chat_message: [...CHAT_LIVE, ['chat', 'search'], ['wa', 'status']], // the numbers carry the unread counts (rail, menu badge)
  wa_status: [['wa']],
  mcp_call: [['mcp']],
  mcp_schedule: [['mcp'], ['chat', 'wa-analysis']], // Claude's Analisa WhatsApp every 2 h: the label in Pusat kendali
  policy_changed: [['policies'], ['agents'], ['mcp']],
  outbox_failed: [['proposals'], ['dealer'], ['plan']],
  routine_fired: [['mcp']],
  // a BigQuery sync finished (full, or the stock alone every few minutes): every page built from it refreshes
  data_synced: [['stock'], ['dealers'], ['dealer'], ['orbit'], ['segmen'], ['kpi'], ['credit'], ['trend'], ['agenda']], // the Claude routine started (or could not) for a waiting cycle — the analysis pop-up shows it
}

/**
 * The queries an event refreshes. A new message names its chat (thread_id): only that chat and its context refetch,
 * with the chat list, search and unread counts — not the chat each tab has open whatever it is (Sam 28 Oct "lemot":
 * every message of 9 numbers refetched every open chat, ~6,000 chat loads an hour).
 */
export function keysFor(name: string, data: unknown): string[][] {
  const thread = name === 'chat_message' && data && typeof data === 'object' ? (data as { thread_id?: unknown }).thread_id : undefined
  if (typeof thread === 'string' && thread) {
    return [['chat', 'threads'], ['chat', 'thread', thread], ['chat', 'context', thread], ['chat', 'search'], ['wa', 'status']]
  }
  return INVALIDATES[name] ?? []
}

/** Collects query keys and invalidates each distinct one once per `wait` ms (leading edge immediate, then trailing). */
export class InvalidationQueue {
  private keys = new Map<string, string[]>()
  private timer: ReturnType<typeof setTimeout> | undefined
  private last = 0
  private run: (key: string[]) => void
  private wait: number
  private now: () => number
  constructor(run: (key: string[]) => void, wait: number, now: () => number = Date.now) {
    this.run = run
    this.wait = wait
    this.now = now
  }

  add(key: string[]) {
    this.keys.set(key.join('\u0000'), key)
    if (this.timer !== undefined) return
    const due = Math.max(0, this.last + this.wait - this.now())
    this.timer = setTimeout(() => this.flush(), due)
  }

  private flush() {
    this.timer = undefined
    this.last = this.now()
    const keys = [...this.keys.values()]
    this.keys.clear()
    keys.forEach((k) => this.run(k))
  }

  dispose() {
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.timer = undefined
    this.keys.clear()
  }
}

type Listener = (event: string, data: unknown) => void
interface SseState { connected: boolean; stale: boolean; subscribe: (l: Listener) => () => void }
const Ctx = createContext<SseState>({ connected: false, stale: false, subscribe: () => () => {} })

export const useSse = () => useContext(Ctx)

// Realtime as far as it goes (Sam 22 Oct: "beberapa chat tidak sinkron dan tidak realtime"). The server stores a
// WhatsApp message within a second (median 0.7 s, 24 h), so what fell behind was the tab: an EventSource that died
// silently (sleep, Wi-Fi change, an idle connection cut), the events missed while it was down, and nothing in their
// place. Now: a watchdog on the server's 25 s heartbeat reconnects a silent stream; every (re)connect, a tab coming
// back and the network coming back refetch what may have changed; while the stream is down the chats poll.
/** No event at all for this long (heartbeat every 25 s): the stream is dead — reconnect. */
export const SILENT_MS = 70_000
/** Polling of the chats while the stream is down. */
export const POLL_MS = 10_000
/** A stream down this long shows "Menyambung ulang…". */
const STALE_MS = 5_000
/** What a reconnect or a tab coming back refetches: what a missed event would have. */
export const CATCH_UP: string[][] = [...CHAT_LIVE, ['chat', 'search'], ['chat', 'starred'], ['wa', 'status'], ['proposals'], ['plan'], ['cycle']]

export function SseProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [connected, setConnected] = useState(false)
  const [stale, setStale] = useState(false)
  const [listeners] = useState(() => new Set<Listener>())
  useEffect(() => {
    if (typeof EventSource === 'undefined') return
    // a burst of events (a number's history arriving: dozens a minute) refetches each affected query once per
    // second, not once per event: the chat list, thread and context were fetched ~5×/s per tab
    const queue = new InvalidationQueue((queryKey) => qc.invalidateQueries({ queryKey }), 1500)
    const catchUp = () => CATCH_UP.forEach((k) => queue.add(k))
    let es: EventSource | null = null
    let lastSeen = Date.now()
    let downSince: number | null = Date.now()
    let everConnected = false
    let closed = false
    const seen = () => { lastSeen = Date.now() }
    const handlers = Object.keys(INVALIDATES).map((name) => [name, (e: MessageEvent) => {
      seen()
      let data: unknown = null
      try {
        data = JSON.parse(e.data)
      } catch {
        data = e.data
      }
      keysFor(name, data).forEach((queryKey) => queue.add(queryKey))
      listeners.forEach((l) => l(name, data))
    }] as const)
    const onHello = () => {
      seen()
      if (everConnected) catchUp() // reconnected: fetch what was missed while down
      everConnected = true
      downSince = null
      setConnected(true)
      setStale(false)
    }
    const down = () => {
      if (downSince === null) downSince = Date.now()
      setConnected(false)
    }
    const open = () => {
      if (closed) return
      es?.close()
      const s = new EventSource('/api/events')
      es = s
      seen()
      s.addEventListener('hello', onHello)
      s.addEventListener('heartbeat', seen)
      s.onerror = () => {
        down()
        // the browser retries by itself unless the stream is closed for good (an HTTP error): then we do
        if (s.readyState === EventSource.CLOSED && es === s) setTimeout(open, 3000)
      }
      handlers.forEach(([n, h]) => s.addEventListener(n, h))
    }
    open()
    // the watchdog: a stream silent past the heartbeat is dead even when the browser thinks it is open; while
    // down, the chats poll and the "Menyambung ulang…" hint shows
    let lastPoll = 0
    const tick = setInterval(() => {
      const now = Date.now()
      if (now - lastSeen > SILENT_MS) {
        down()
        open()
      }
      if (downSince !== null) {
        if (now - downSince > STALE_MS) setStale(true)
        if (now - lastPoll >= POLL_MS) {
          lastPoll = now
          CHAT_LIVE.forEach((k) => queue.add(k))
        }
      }
    }, 5000)
    // a tab coming back (sleep, another app) or the network coming back: refetch, and reconnect a silent stream
    let hiddenAt: number | null = null
    const back = () => {
      if (Date.now() - lastSeen > 30_000 || (es && es.readyState === EventSource.CLOSED)) open()
      catchUp()
    }
    const onVisibility = () => {
      if (document.visibilityState === 'hidden') {
        hiddenAt = Date.now()
        return
      }
      if (hiddenAt !== null && Date.now() - hiddenAt > 10_000) back()
      hiddenAt = null
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('online', back)
    return () => {
      closed = true
      clearInterval(tick)
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('online', back)
      es?.close()
      queue.dispose()
    }
  }, [qc, listeners])
  return (
    <Ctx.Provider value={{ connected, stale, subscribe: (l) => { listeners.add(l); return () => listeners.delete(l) } }}>
      {children}
    </Ctx.Provider>
  )
}
