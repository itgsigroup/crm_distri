import { createContext, useCallback, useContext, useEffect, useReducer, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from '../api/client'
import type { Cycle, CycleLatest, StageName } from '../api/types'
import { useFeedback } from '../components/feedback'
import { hhmm } from '../lib/format'
import { STAGE_LABEL } from '../lib/i18n/id'
import { useSse } from './SseProvider'
import { useCycleLatest, useQueue } from './queries'
import { STAGES, TICK_MS, cycleReducer, initialCycle, type CycleUI } from './cycle'

/** Label of a scope string, as the API's domain.Scope.Label ("screen:orbit" → "orbit"). */
export function scopeLabel(scope: string): string {
  if (!scope || scope === 'all') return 'semua'
  const [kind, id = ''] = scope.split(':')
  if (kind === 'screen') return ({ orbit: 'orbit', segmen: 'segmen', stock: 'push stok', credit: 'kredit', relasi: 'peta relasi', dealer: 'dealer' } as Record<string, string>)[id] ?? id
  if (kind === 'agent') return id
  return 'dealer ' + id
}

type Via = 'auto' | 'api' | 'mcp'
interface OrchCtx {
  s: CycleUI
  reanalyze: (scope: string, via?: Via) => void
}
const Ctx = createContext<OrchCtx>({ s: initialCycle, reanalyze: () => {} })

/** Owns the live cycle state of the tab: POST /cycles, SSE events, a polling fallback and the finishing toast. */
export function OrchProvider({ children }: { children: ReactNode }) {
  const [s, dispatch] = useReducer(cycleReducer, initialCycle)
  const { subscribe } = useSse()
  const { toast, alert } = useFeedback()
  const qc = useQueryClient()

  useEffect(
    () =>
      subscribe((name, data) => {
        const d = (data ?? {}) as Record<string, unknown>
        if (name === 'cycle_stage') {
          dispatch({ type: 'stage', cycleId: String(d.cycle_id), stage: d.stage as StageName, status: String(d.status), label: scopeLabel(String(d.scope ?? 'all')) })
        } else if (name === 'cycle_done') {
          dispatch({ type: 'done', cycleId: String(d.id), result: { number: (d.number as number) ?? null, note: String(d.note ?? ''), updated: Number(d.updated ?? 0), label: String(d.label ?? 'semua'), via: String(d.via ?? 'api'), status: String(d.status ?? 'done') } })
        }
      }),
    [subscribe],
  )

  useEffect(() => {
    if (!s.running) return
    const t = setInterval(() => dispatch({ type: 'tick' }), TICK_MS)
    return () => clearInterval(t)
  }, [s.running])

  // without SSE (proxy, network) the cycle still finishes in the UI
  useEffect(() => {
    if (!s.running || s.finished || !s.cycleId) return
    const id = s.cycleId
    const t = setInterval(async () => {
      try {
        const l = await api.get<CycleLatest>('/cycles/latest')
        const c = l.cycle
        if (!c || c.id !== id) return
        if (c.stage) dispatch({ type: 'stage', cycleId: id, stage: c.stage, status: 'running' })
        if (c.status === 'done' || c.status === 'partial' || c.status === 'failed') {
          dispatch({ type: 'done', cycleId: id, result: { number: c.number, note: c.note ?? '', updated: 0, label: c.label, via: c.via ?? 'api', status: c.status } })
        }
      } catch {
        // keep waiting for SSE
      }
    }, 1500)
    return () => clearInterval(t)
  }, [s.running, s.finished, s.cycleId])

  useEffect(() => {
    if (s.running || !s.result) return
    const r = s.result
    if (s.mine) {
      const at = `${hhmm(new Date())} WIB`
      const cycle = r.number ? `#${r.number.toLocaleString('id-ID')}` : '—'
      if (r.status === 'failed') {
        alert({ icon: 'error', title: 'Analisis gagal', text: `Analisis ulang ${r.label} tidak selesai. Data dan saran sebelumnya tetap dipakai; coba lagi beberapa saat lagi.`, details: [['Siklus', cycle], ['Waktu', at], ['Penyebab', r.note || 'tidak diketahui']] })
      } else {
        alert({
          icon: r.status === 'partial' ? 'warning' : 'success',
          title: r.status === 'partial' ? 'Analisis selesai sebagian' : 'Analisis berhasil',
          text: r.status === 'partial' ? `Analisis ulang ${r.label} selesai, tetapi sebagian agen tidak berjalan.` : `Analisis ulang ${r.label} selesai lewat ${r.via.toUpperCase()}.`,
          details: [['Siklus', cycle], ['Saran diperbarui', String(r.updated)], ['Selesai', at], ...(r.note ? [['Catatan', r.note] as [string, string]] : [])],
        })
      }
    }
    dispatch({ type: 'ack' })
    for (const k of ['cycle', 'cycles', 'plan', 'proposals', 'agents', 'brief', 'dealers', 'dealer', 'orbit', 'segmen']) qc.invalidateQueries({ queryKey: [k] })
  }, [s.running, s.result, s.mine, alert, qc])

  const reanalyze = useCallback(
    (scope: string, via: Via = 'auto') => {
      if (s.running) {
        toast('Orchestrator sedang berjalan')
        return
      }
      const v = via === 'mcp' ? 'mcp' : 'api'
      api
        .post<Cycle>('/cycles', { scope, via: v })
        .then((c) => dispatch({ type: 'start', cycleId: c.id, label: c.label, via: v }))
        .catch((e: unknown) =>
          e instanceof ApiError && e.status === 409
            ? alert({ icon: 'warning', title: 'Analisis sedang berjalan', text: 'Orchestrator masih menganalisis. Tunggu sampai selesai, lalu coba lagi.' })
            : alert({ icon: 'error', title: 'Analisis gagal dimulai', text: e instanceof Error ? e.message : 'Gagal memulai analisis', details: [['Waktu', `${hhmm(new Date())} WIB`]] }),
        )
    },
    [s.running, toast, alert],
  )

  return <Ctx.Provider value={{ s, reanalyze }}>{children}</Ctx.Provider>
}

export const useOrch = () => useContext(Ctx)

// Orchestrator status shared by the rail, topbar pill, card and dock.
export interface OrchStatus {
  running: boolean
  stage: string
  scope: string
  run: number | null
  last: string | null
  next: string
  dur: string
  pending: number
  unread: number
  hasCycle: boolean
  lastDone: Cycle | null
  lastFull: Cycle | null
}

export function nextRun(now: Date): string {
  const t = new Date(now.getTime())
  t.setMinutes(0, 0, 0)
  t.setHours(t.getHours() + 1)
  const h = Number(hhmm(t).slice(0, 2))
  if (h < 6 || h > 20) return '06.00'
  return hhmm(t)
}

/** "2 mnt 14 dtk" / "22 dtk" (mockup ORCH.dur). */
export function duration(ms: number | null | undefined): string {
  if (ms == null) return '—'
  const sec = Math.max(1, Math.round(ms / 1000))
  return sec >= 60 ? `${Math.floor(sec / 60)} mnt ${sec % 60} dtk` : `${sec} dtk`
}

export function useOrchStatus(): OrchStatus {
  const { s } = useOrch()
  const { data: latest } = useCycleLatest()
  const { data: queue = [] } = useQueue()
  const last = latest?.last_done ?? null
  const running = s.running || !!latest?.running
  const shownStage = s.running ? STAGES[Math.min(s.shown, STAGES.length - 1)] : (latest?.cycle?.stage ?? 'ingest')
  return {
    running,
    stage: STAGE_LABEL[shownStage],
    scope: s.running ? s.label : latest?.cycle?.label ?? 'semua',
    run: last?.number ?? null,
    last: last ? hhmm(last.started_at) : null,
    next: latest ? hhmm(latest.next_at) : '06.00',
    dur: duration(last?.duration_ms),
    pending: queue.filter((q) => q.status === 'proposed').length,
    unread: 0,
    hasCycle: !!last,
    lastDone: last,
    lastFull: latest?.last_full ?? null,
  }
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
