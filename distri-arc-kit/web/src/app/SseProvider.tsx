import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

// One EventSource('/api/events') per tab; each event invalidates the queries it affects (08-frontend).
const INVALIDATES: Record<string, string[][]> = {
  cycle_stage: [['cycle']],
  cycle_done: [['cycle'], ['cycles'], ['plan'], ['proposals'], ['brief'], ['orbit'], ['segmen'], ['dealers'], ['dealer'], ['kpi'], ['agenda'], ['agents'], ['stock'], ['credit'], ['chat', 'wa-analysis']],
  proposal_changed: [['proposals'], ['plan'], ['dealer'], ['dealers'], ['cycle'], ['orbit'], ['chat']],
  chat_message: [['chat'], ['wa', 'status']], // the numbers carry the unread counts (rail, menu badge)
  wa_status: [['wa']],
  mcp_call: [['mcp']],
  mcp_schedule: [['mcp'], ['chat', 'wa-analysis']], // Claude's Analisa WhatsApp every 2 h: the label in Pusat kendali
  policy_changed: [['policies'], ['agents'], ['mcp']],
  outbox_failed: [['proposals'], ['dealer'], ['plan']],
  routine_fired: [['mcp']],
  // a BigQuery sync finished (full, or the stock alone every few minutes): every page built from it refreshes
  data_synced: [['stock'], ['dealers'], ['dealer'], ['orbit'], ['segmen'], ['kpi'], ['credit'], ['trend'], ['agenda']], // the Claude routine started (or could not) for a waiting cycle — the analysis pop-up shows it
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
const Ctx = createContext<{ connected: boolean; subscribe: (l: Listener) => () => void }>({ connected: false, subscribe: () => () => {} })

export const useSse = () => useContext(Ctx)

export function SseProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [connected, setConnected] = useState(false)
  const [listeners] = useState(() => new Set<Listener>())
  useEffect(() => {
    if (typeof EventSource === 'undefined') return
    const es = new EventSource('/api/events')
    // a burst of events (a number's history arriving: dozens a minute) refetches each affected query once per
    // second, not once per event: the chat list, thread and context were fetched ~5×/s per tab
    const queue = new InvalidationQueue((queryKey) => qc.invalidateQueries({ queryKey }), 1500)
    es.addEventListener('hello', () => setConnected(true))
    es.onerror = () => setConnected(false)
    const handlers = Object.keys(INVALIDATES).map((name) => {
      const h = (e: MessageEvent) => {
        let data: unknown = null
        try {
          data = JSON.parse(e.data)
        } catch {
          data = e.data
        }
        INVALIDATES[name].forEach((queryKey) => queue.add(queryKey))
        listeners.forEach((l) => l(name, data))
      }
      es.addEventListener(name, h)
      return [name, h] as const
    })
    return () => {
      handlers.forEach(([n, h]) => es.removeEventListener(n, h))
      es.close()
      queue.dispose()
    }
  }, [qc, listeners])
  return (
    <Ctx.Provider value={{ connected, subscribe: (l) => { listeners.add(l); return () => listeners.delete(l) } }}>
      {children}
    </Ctx.Provider>
  )
}
