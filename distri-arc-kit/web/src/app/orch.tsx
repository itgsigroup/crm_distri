import { createContext, useCallback, useContext, useEffect, useMemo, useReducer, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from '../api/client'
import type { Cycle, CycleLatest, StageName } from '../api/types'
import { useFeedback } from '../components/feedback'
import { hhmm, shortDate } from '../lib/format'
import { STAGE_LABEL } from '../lib/i18n/id'
import { useSse } from './SseProvider'
import { useCycleLatest, useMe, useNow, useQueue, useWAAnalysis } from './queries'
import { STAGES, TICK_MS, cycleReducer, initialCycle, type CycleUI } from './cycle'
import { AnalysisSheet, type QueuedCycle } from './AnalysisSheet'
import { effort, mcpModel } from './enginePrefs'

/** Label of a scope string, as the API's domain.Scope.Label ("screen:orbit" → "orbit"). */
export function scopeLabel(scope: string): string {
  if (!scope || scope === 'all') return 'semua'
  const [kind, id = ''] = scope.split(':')
  if (kind === 'screen') return ({ orbit: 'orbit', segmen: 'segmen', stock: 'push stok', credit: 'kredit', relasi: 'peta relasi', dealer: 'dealer', chat: 'chat' } as Record<string, string>)[id] ?? id
  if (kind === 'agent') return id
  if (kind === 'thread') return 'chat ini'
  return 'dealer ' + id
}

/** The engine label of a cycle run by GSI Orbit itself: rules and formulas, no AI (ADR 0046). */
export const SYSTEM_LABEL = 'Sistem GSI Orbit'

type Via = 'auto' | 'api' | 'mcp' | 'model' | 'system'
interface OrchCtx {
  s: CycleUI
  /** engine: who analyses ('mcp' Claude through MCP, 'model' the API & model AI choice); label names it in the result;
   * inline: the page shows the analysis itself (the Orchestrator page), no pop-up */
  reanalyze: (scope: string, via?: Via, label?: string, inline?: boolean) => void
  /** the last cycle this tab asked for, with how it is analysed (the inline panel's Claude button) */
  queued: QueuedCycle | null
  /** an analysis panel is on screen and announces the result itself (no pop-up); returns the unregister */
  showPanel: () => () => void
}
const NONE: OrchCtx = { s: initialCycle, reanalyze: () => {}, queued: null, showPanel: () => () => {} }
const Ctx = createContext<OrchCtx>(NONE)

// The analysis pop-up is rendered by FeedbackProvider, which sits above OrchProvider (OrchProvider needs its toast and
// sheet): there useContext only sees NONE. OrchProvider publishes its live value here so the pop-up follows the cycle
// (19 Oct: it showed "Selesai" / "API" with every stage ticked while the cycle was still running).
let live: OrchCtx = NONE
const listeners = new Set<() => void>()
const subscribeLive = (l: () => void) => {
  listeners.add(l)
  return () => { listeners.delete(l) }
}
const getLive = () => live

/** Everything a finished cycle can change, so the open page shows the new analysis (proposals, page readings,
 * metrics, Analisa WhatsApp, brief) — also when SSE is down and the poll ends the cycle. */
export function refreshPages(qc: ReturnType<typeof useQueryClient>) {
  for (const k of ['cycle', 'cycles', 'plan', 'proposals', 'agents', 'brief', 'dealers', 'dealer', 'orbit', 'segmen', 'relasi', 'kpi', 'agenda', 'stock', 'credit', 'trend'])
    qc.invalidateQueries({ queryKey: [k] })
  qc.invalidateQueries({ queryKey: ['chat', 'wa-analysis'] })
}

/** Owns the live cycle state of the tab: POST /cycles, SSE events, a polling fallback and the finishing toast. */
export function OrchProvider({ children }: { children: ReactNode }) {
  const [s, dispatch] = useReducer(cycleReducer, initialCycle)
  const [queued, setQueued] = useState<QueuedCycle | null>(null)
  const panels = useRef(0) // analysis panels on screen (they announce the result instead of the pop-up)
  const showPanel = useCallback(() => {
    panels.current++
    return () => { panels.current-- }
  }, [])
  const { subscribe } = useSse()
  const { toast, alert, openSheet } = useFeedback()
  const qc = useQueryClient()

  useEffect(
    () =>
      subscribe((name, data) => {
        const d = (data ?? {}) as Record<string, unknown>
        if (name === 'cycle_stage') {
          dispatch({ type: 'stage', cycleId: String(d.cycle_id), stage: d.stage as StageName, status: String(d.status), label: scopeLabel(String(d.scope ?? 'all')), at: Date.now() })
        } else if (name === 'cycle_done') {
          // a page reading or Analisa WhatsApp sent through MCP (source mcp, agent set) is not the end of the cycle
          if (d.agent) return
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
        let c = l.cycle
        // another cycle is the latest now: read the one this tab follows, so a missed cycle_done does not leave the
        // app "running" (every Analisis ulang / Jalankan ulang button stayed disabled, 20 Oct)
        if (c && c.id !== id) c = (await api.get<{ cycle: Cycle }>(`/cycles/${id}`)).cycle
        if (!c) return
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
    if (s.mine && panels.current === 0) { // the Pusat kendali panel shows the result itself
      const at = `${hhmm(new Date())} WIB`
      const cycle = r.number ? `#${r.number.toLocaleString('id-ID')}` : '—'
      if (r.status === 'failed') {
        alert({ icon: 'error', title: 'Analisis gagal', text: `Analisis ulang ${r.label} tidak selesai. Data dan saran sebelumnya tetap dipakai; coba lagi beberapa saat lagi.`, details: [['Siklus', cycle], ['Waktu', at], ['Penyebab', r.note || 'tidak diketahui']] })
      } else {
        alert({
          icon: r.status === 'partial' ? 'warning' : 'success',
          title: r.status === 'partial' ? 'Analisis selesai sebagian' : 'Analisis berhasil',
          text: r.status === 'partial' ? `Analisis ulang ${r.label} selesai, tetapi sebagian agen tidak berjalan.` : `Analisis ulang ${r.label} selesai lewat ${s.via && s.via !== 'api' ? s.via : r.via.toUpperCase()}. Halaman ini sudah diperbarui dengan hasilnya.`,
          details: [['Siklus', cycle], ['Saran diperbarui', String(r.updated)], ['Selesai', at], ...(r.note ? [['Catatan', r.note] as [string, string]] : [])],
        })
      }
    }
    dispatch({ type: 'ack', at: Date.now() })
    refreshPages(qc)
  }, [s.running, s.result, s.mine, s.via, alert, qc])

  const reanalyze = useCallback(
    (scope: string, via: Via = 'auto', label?: string, inline = false) => {
      if (s.running) {
        toast('Orchestrator sedang berjalan')
        return
      }
      const v = via === 'mcp' || via === 'model' || via === 'system' ? via : 'api'
      // the model (for claude.ai) and effort chosen on the dock, the same for every "Analisis ulang"
      api
        .post<QueuedCycle>('/cycles', { scope, via: v, model: mcpModel(), effort: effort() })
        .then((c) => {
          const eff = c.mcp?.effort || c.effort || effort()
          const by = c.mcp?.engine === 'claude_api' ? `${c.mcp.model || label || 'API'} · ${eff}`
            : c.mcp ? `MCP Claude · ${c.mcp.model_label || 'Claude'} · ${eff}` : v === 'system' ? SYSTEM_LABEL : label || 'API'
          dispatch({ type: 'start', cycleId: c.id, label: c.label, via: by, at: Date.now() })
          setQueued(c)
          // Sam 22 Oct: no waiting screen — the analysis runs in the background (topbar pill, Orchestrator page) and the
          // result pops up when done; the pop-up opens only when Claude must be opened by hand to start it
          if (inline) return
          if (c.mcp?.engine === 'claude_ai' && !c.mcp.routine) openSheet(<AnalysisSheet cycle={c} />)
          else toast('Analisis berjalan di belakang layar — hasilnya muncul saat selesai')
        })
        .catch((e: unknown) =>
          e instanceof ApiError && e.code === 'no_api'
            ? alert({ icon: 'warning', title: 'API belum siap', text: e.message, details: [['Atur di', 'Pengaturan → API & model AI']] })
            : e instanceof ApiError && e.status === 409
            ? alert({ icon: 'warning', title: 'Analisis sedang berjalan', text: 'Orchestrator masih menganalisis. Tunggu sampai selesai, lalu coba lagi.' })
            : alert({ icon: 'error', title: 'Analisis gagal dimulai', text: e instanceof Error ? e.message : 'Gagal memulai analisis', details: [['Waktu', `${hhmm(new Date())} WIB`]] }),
        )
    },
    [s.running, toast, alert, openSheet],
  )

  const value = useMemo(() => ({ s, reanalyze, queued, showPanel }), [s, reanalyze, queued, showPanel])
  useEffect(() => {
    live = value
    listeners.forEach((l) => l())
  }, [value])
  useEffect(() => () => {
    live = NONE
    listeners.forEach((l) => l())
  }, [])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

/** The live Orchestrator state: from the provider, or — in the pop-up above it — from what the provider published. */
export function useOrch(): OrchCtx {
  const inTree = useContext(Ctx)
  const outside = useSyncExternalStore(subscribeLive, getLive)
  return inTree === NONE ? outside : inTree
}

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
  const { data: me } = useMe()
  // the top-right pill is the automatic Analisa WhatsApp through MCP (Sam 19 Oct): its last success and next run;
  // users without the chat page keep the hourly Orchestrator cycle
  const canWA = !!me?.screens.includes('chat')
  const { data: wa } = useWAAnalysis(undefined, canWA)
  const today = shortDate(useNow().toISOString())
  const c = wa?.claude
  if (canWA && c) {
    const running = c.last?.status === 'running'
    const ok = c.last_ok_at ?? (c.last?.status === 'ok' ? c.last.finished_at ?? c.last.started_at : null)
    const failed = !running && c.last && c.last.status !== 'ok'
    const day = (d: string) => (shortDate(d) === today ? hhmm(d) : `${shortDate(d)} ${hhmm(d)}`)
    return (
      <button className={`orch-pill wa${running ? ' running' : ''}${failed ? ' bad' : ''}`} onClick={() => nav('/')}
        title={`Analisa WhatsApp otomatis lewat MCP${c.runner === 'external' ? ' · routine claude.ai' : ' · API server'} · ${c.cron}${failed && c.last?.error ? ` · gagal: ${c.last.error}` : ''}`}>
        <span className="op-dot" />
        <span className="op-t">Analisa WhatsApp · MCP <b>{running ? 'berjalan…' : ok ? day(ok) : 'belum ada'}</b>{failed && <span className="op-bad"> · gagal</span>}</span>
        <span className="op-next">berikutnya <b>{c.enabled && c.next_run_at ? hhmm(c.next_run_at) : 'dijeda'}</b></span>
      </button>
    )
  }
  return (
    <button className="orch-pill" onClick={() => nav('/orchestrator')}>
      <span className="op-dot" />
      <span className="op-t">Analisis <b>{s.run ? '#' + s.run.toLocaleString('id-ID') : '#—'}</b> · <span>{s.last ?? 'belum ada'}</span></span>
      <span className="op-next">berikutnya <b>{s.next}</b></span>
    </button>
  )
}
