import { useNavigate } from 'react-router'
import { useNow, useQueue } from './queries'
import { hhmm } from '../lib/format'

// Orchestrator status shared by the rail, topbar pill and dock. Until stage 06 there is no cycle yet:
// the status is "siap", nothing pending, and the next run is the next full hour within 06.00–20.00 WIB.
export interface OrchStatus {
  running: boolean
  run: number | null
  last: string | null
  next: string
  pending: number
  unread: number
}

export function nextRun(now: Date): string {
  const t = new Date(now.getTime())
  t.setMinutes(0, 0, 0)
  t.setHours(t.getHours() + 1)
  const h = Number(hhmm(t).slice(0, 2))
  if (h < 6 || h > 20) return '06.00'
  return hhmm(t)
}

export function useOrchStatus(): OrchStatus {
  const now = useNow()
  const { data: queue = [] } = useQueue()
  return { running: false, run: null, last: null, next: nextRun(now), pending: queue.filter((q) => q.status === 'proposed').length, unread: 0 }
}

export function OrchPill() {
  const s = useOrchStatus()
  const nav = useNavigate()
  return (
    <button className="orch-pill" onClick={() => nav('/orchestrator')}>
      <span className="op-dot" />
      <span className="op-t">Analisis <b>{s.run ? '#' + s.run.toLocaleString('id-ID') : '#—'}</b> · <span>{s.last ?? 'belum ada'}</span></span>
      <span className="op-next">berikutnya <b>{s.next}</b></span>
    </button>
  )
}
