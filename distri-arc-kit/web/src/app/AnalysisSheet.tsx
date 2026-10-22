import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Cycle } from '../api/types'
import { Icon } from '../components/Icon'
import { SheetHead, useFeedback } from '../components/feedback'
import { AnalysisResult, AnalysisRunning, CancelButton, READING } from '../features/control/AnalysisPanel'
import { summarize, type CycleProgress } from '../features/control/progress'
import { STAGES } from './cycle'
import { duration, useOrch } from './orch'
import { useSse } from './SseProvider'

/** How a cycle asked from the "Analisis ulang" button is analysed by Claude through MCP (POST /cycles → mcp). */
export interface CycleMCP { engine: 'claude_api' | 'claude_ai' | 'chatgpt_api'; /** the Claude routine on claude.ai runs it (ADR 0049) */ routine?: boolean; /** claude_api: "Claude API · Claude Opus 5.5" */ model?: string; prompt: string; claude_url: string; wait_sec: number; agents: string[]; /** the API second option (ADR 0040), "" when off */ fallback?: string; /** the model and effort asked for */ model_label?: string; effort?: string }
export type QueuedCycle = Cycle & { mcp?: CycleMCP }

interface InputState { agent: string; submitted_at: string | null }

/** The live part of an analysis started with "Analisis ulang": "AI sedang membaca data…" with the progress while it
 * runs (through claude.ai with one button to open Claude — Claude only works when someone sends it
 * there), then berhasil / gagal. Shown in the pop-up and inline on the Orchestrator page. */
export function AnalysisLive({ cycle }: { cycle: QueuedCycle }) {
  const { s, showPanel } = useOrch()
  useEffect(() => showPanel(), [showPanel])
  const last = s.last?.cycleId === cycle.id ? s.last : null
  const hint = useClaudeHint(cycle)
  return last ? <AnalysisResult last={last} /> : <AnalysisRunning extra={hint} />
}

/** The analysis inside the Orchestrator dock (Sam 22 Oct: no second pop-up, the progress in the first): the stage
 * running with its time, the agents done, Claude's hint and Batalkan — then one line berhasil / gagal. */
export function AnalysisCompact({ cycle }: { cycle: QueuedCycle }) {
  const { s, showPanel } = useOrch()
  useEffect(() => showPanel(), [showPanel])
  const [now, setNow] = useState(() => Date.now())
  const running = s.cycleId === cycle.id && s.running
  useEffect(() => {
    if (!running) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [running])
  const { data: p } = useQuery({
    queryKey: ['cycle-progress', cycle.id],
    queryFn: () => api.get<CycleProgress>(`/cycles/${cycle.id}/progress`),
    refetchInterval: running ? 2500 : false,
  })
  const hint = useClaudeHint(cycle)
  const last = s.last?.cycleId === cycle.id ? s.last : null
  if (last) {
    const failed = last.status === 'failed'
    const partial = last.status === 'partial'
    return (
      <div className={`dk-prog ${failed ? 'bad' : partial ? 'warn' : 'good'}`} role="status">
        <div className="dk-prog-h">
          <Icon name={failed ? 'alert' : 'check'} />
          <b>{failed ? 'Analisis gagal' : partial ? 'Analisis selesai sebagian' : 'Analisis berhasil'}</b>
          <span>{last.number ? `#${last.number.toLocaleString('id-ID')} · ` : ''}{last.startedAt ? duration(last.endedAt - last.startedAt) : ''}</span>
        </div>
        {last.note && <small>{failed ? 'Penyebab' : 'Catatan'}: {last.note}</small>}
        {!failed && <small>{last.updated} saran diperbarui · halaman sudah memakai hasilnya</small>}
      </div>
    )
  }
  const cur = READING[STAGES[Math.min(s.shown, STAGES.length - 1)]]
  const sum = p ? summarize(p) : null
  return (
    <div className="dk-prog run" role="status" aria-live="polite">
      <div className="dk-prog-h">
        <i className="an-spin" />
        <b>{cur.name}</b>
        <span>{s.startedAt ? duration(now - s.startedAt) : ''}</span>
      </div>
      <small>{sum?.now || cur.line}{sum && sum.total > 0 ? ` · ${sum.done}/${sum.total} agen selesai` : ''}</small>
      {hint}
      {s.cycleId && <CancelButton cycleId={s.cycleId} />}
    </div>
  )
}

/** How Claude takes the cycle: routine, by hand (with "Buka Claude"), or the API — and how many agents it sent. */
function useClaudeHint(cycle: QueuedCycle) {
  const { s } = useOrch()
  const m = cycle.mcp
  const mine = s.cycleId === cycle.id
  const running = mine && s.running
  const { data: inputs = [] } = useQuery({
    queryKey: ['cycle', cycle.id, 'inputs'],
    queryFn: () => api.get<{ items: InputState[] }>(`/cycles/${cycle.id}/inputs`).then((r) => r.items),
    enabled: !!m,
    refetchInterval: running ? 3000 : false,
  })
  const sent = inputs.filter((x) => x.submitted_at).length
  // Claude through MCP analyses it: the Claude routine on claude.ai when set (ADR 0049) — nobody opens anything —
  // else in claude.ai / Desktop with the prompt
  const claude = m?.engine === 'claude_ai'
  const routine = claude && !!m?.routine
  const [fired, setFired] = useState<{ ok: boolean; url?: string; error?: string } | null>(null)
  const { subscribe } = useSse()
  useEffect(() => subscribe((name, data) => {
    const d = (data ?? {}) as { cycle_id?: string; ok?: boolean; url?: string; error?: string }
    if (name === 'routine_fired' && d.cycle_id === cycle.id) setFired({ ok: !!d.ok, url: d.url, error: d.error })
  }), [subscribe, cycle.id])
  const byHand = claude && sent === 0 && (!routine || (fired && !fired.ok))
  const hint = m && (
    <div className="an-mcp">
      <span>
        {sent > 0 ? <>Claude sudah mengirim <b>{sent}/{m.agents.length}</b> agen.</>
          : routine && fired?.ok ? <>Claude sedang bekerja di claude.ai (routine) — tidak perlu membuka apa pun. Tanpa kunci API, tanpa ChatGPT.</>
          : routine && fired ? <>Routine Claude gagal dijalankan: {fired.error}. Buka Claude sendiri untuk melanjutkan.</>
          : routine ? <>Claude akan menganalisa otomatis lewat routine claude.ai saat data siap — tidak perlu membuka apa pun.</>
          : claude ? <>Claude membaca lewat MCP: buka Claude, pilih <b>{m.model_label || 'Claude'}</b>{m.effort ? <> · effort <b>{m.effort}</b></> : null}, lalu tekan Enter. Tanpa kunci API, tanpa ChatGPT. Agar tidak perlu membuka Claude, isi <b>Routine Claude</b> di halaman MCP Claude.</>
          : <><b>{m.model || 'API'}</b> membaca di server — tidak perlu membuka apa pun.</>}
      </span>
      {routine && fired?.ok && fired.url && <a className="btn ghost" href={fired.url} target="_blank" rel="noopener noreferrer"><Icon name="spark" />Lihat Claude bekerja</a>}
      {byHand && <a className="btn primary" href={m.claude_url} target="_blank" rel="noopener noreferrer"><Icon name="plug" />Buka Claude</a>}
    </div>
  )
  return hint
}

/** The analysis on the page itself (Orchestrator, Dealer — Sam 17 Oct): while a cycle runs, what the AI analyses with
 * the progress; after the one asked here, berhasil / gagal until closed. onlyMine: a page that shows only
 * the analysis it asked for (the dealer page), not the hourly cycle. */
export function InlineAnalysis({ onlyMine = false }: { onlyMine?: boolean }) {
  const { s, queued } = useOrch()
  const [closed, setClosed] = useState<string | null>(null)
  const ours = queued && (s.cycleId === queued.id || s.last?.cycleId === queued.id) ? queued : null
  if (ours && closed !== ours.id && (s.running || s.last?.cycleId === ours.id)) {
    return (
      <div className={`card an-panel ${s.running ? 'running' : ''}`} role="status" aria-live="polite">
        <AnalysisLive cycle={ours} />
        {!s.running && <div className="an-close"><button className="btn quiet" onClick={() => setClosed(ours.id)}>Tutup</button></div>}
      </div>
    )
  }
  if (!s.running || onlyMine) return null
  return <div className="card an-panel running" role="status" aria-live="polite"><AnalysisRunning /></div>
}

/** The pop-up of an analysis started with "Analisis ulang" (Sam 16 Oct: in place of the MCP steps). It announces the
 * result itself (no alert). */
export function AnalysisSheet({ cycle }: { cycle: QueuedCycle }) {
  const { closeSheet } = useFeedback()
  const { s, minimize } = useOrch()
  const running = s.cycleId === cycle.id && s.running
  const done = s.last?.cycleId === cycle.id
  const title = cycle.number ? `Analisis ulang ${cycle.label} · siklus #${cycle.number.toLocaleString('id-ID')}` : `Analisis ulang ${cycle.label}`
  return (
    <>
      <SheetHead icon="spark" title={title} sub={running ? 'Halaman ini boleh ditutup — analisis tetap berjalan dan hasilnya tersimpan' : done ? 'Selesai' : 'Menunggu giliran — siklus lain sedang berjalan'} onClose={closeSheet} />
      <div className="sec an-sheet">
        <AnalysisLive cycle={cycle} />
      </div>
      <div className="ft">
        {running
          ? <button className="btn ghost" onClick={() => { minimize(cycle); closeSheet() }} title="Pop-up jadi bar kecil di pojok; analisis tetap berjalan"><Icon name="chev" style={{ transform: 'rotate(90deg)' }} />Perkecil · jalan di belakang layar</button>
          : <button className="btn quiet" onClick={closeSheet}>Tutup</button>}
        <span className="spacer" /><span className="pol"><Icon name="lock" />AI hanya mengusulkan · keputusan tetap di aplikasi</span></div>
    </>
  )
}
