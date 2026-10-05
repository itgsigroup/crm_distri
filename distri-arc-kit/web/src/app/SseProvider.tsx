import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

// One EventSource('/api/events') per tab; each event invalidates the queries it affects (08-frontend).
const INVALIDATES: Record<string, string[][]> = {
  cycle_stage: [['cycle']],
  cycle_done: [['cycle'], ['cycles'], ['plan'], ['proposals'], ['brief'], ['orbit'], ['segmen'], ['dealers'], ['dealer'], ['kpi'], ['agenda'], ['agents']],
  proposal_changed: [['proposals'], ['plan'], ['dealer'], ['dealers'], ['cycle']],
  chat_message: [['chat']],
  wa_status: [['wa']],
  mcp_call: [['mcp']],
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
        INVALIDATES[name].forEach((queryKey) => qc.invalidateQueries({ queryKey }))
        listeners.forEach((l) => l(name, data))
      }
      es.addEventListener(name, h)
      return [name, h] as const
    })
    return () => {
      handlers.forEach(([n, h]) => es.removeEventListener(n, h))
      es.close()
    }
  }, [qc, listeners])
  return (
    <Ctx.Provider value={{ connected, subscribe: (l) => { listeners.add(l); return () => listeners.delete(l) } }}>
      {children}
    </Ctx.Provider>
  )
}
