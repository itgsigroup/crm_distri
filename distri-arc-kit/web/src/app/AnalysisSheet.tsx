import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Cycle } from '../api/types'
import { Icon } from '../components/Icon'
import { SheetHead, useFeedback } from '../components/feedback'
import { AnalysisResult, AnalysisRunning } from '../features/control/AnalysisPanel'
import { useOrch } from './orch'

/** How a cycle asked from the "Analisis ulang" button is analysed by Claude through MCP (POST /cycles → mcp). */
export interface CycleMCP { engine: 'claude_api' | 'claude_ai'; /** claude_api: "Claude API · Claude Opus 5.5" */ model?: string; prompt: string; claude_url: string; wait_sec: number; agents: string[]; /** the API second option (ADR 0040), "" when off */ fallback?: string; /** the model and effort asked for */ model_label?: string; effort?: string }
export type QueuedCycle = Cycle & { mcp?: CycleMCP }

interface InputState { agent: string; submitted_at: string | null }

/** The live part of an analysis started with "Analisis ulang": "AI sedang membaca data…" with the progress and the
 * game while it runs (through claude.ai with one button to open Claude — Claude only works when someone sends it
 * there), then berhasil / gagal. Shown in the pop-up and inline on the Orchestrator page. */
export function AnalysisLive({ cycle }: { cycle: QueuedCycle }) {
  const { s, showPanel } = useOrch()
  useEffect(() => showPanel(), [showPanel])
  const m = cycle.mcp
  const mine = s.cycleId === cycle.id
  const running = mine && s.running
  const last = s.last?.cycleId === cycle.id ? s.last : null
  const { data: inputs = [] } = useQuery({
    queryKey: ['cycle', cycle.id, 'inputs'],
    queryFn: () => api.get<{ items: InputState[] }>(`/cycles/${cycle.id}/inputs`).then((r) => r.items),
    enabled: !!m,
    refetchInterval: running ? 3000 : false,
  })
  const sent = inputs.filter((x) => x.submitted_at).length
  // Claude through MCP analyses it (claude.ai / Desktop) — the prompt opens there
  const claude = m?.engine === 'claude_ai'
  const who = 'Claude'
  const hint = m && (
    <div className="an-mcp">
      <span>
        {sent > 0 ? <>{who} sudah mengirim <b>{sent}/{m.agents.length}</b> agen.</>
          : claude ? <>{who} membaca lewat MCP: buka {who}, pilih <b>{m.model_label || who}</b>{m.effort ? <> · effort <b>{m.effort}</b></> : null}, lalu tekan Enter. Tanpa kunci API, tanpa ChatGPT.</>
          : <><b>{m.model || 'API'}</b> membaca di server — tidak perlu membuka apa pun.</>}
      </span>
      {claude && sent === 0 && <a className="btn primary" href={m.claude_url} target="_blank" rel="noopener noreferrer"><Icon name="plug" />Buka {who}</a>}
    </div>
  )
  return last ? <AnalysisResult last={last} /> : <AnalysisRunning extra={hint} focusGame={!claude} />
}

/** The analysis on the page itself (Orchestrator, Dealer — Sam 17 Oct): while a cycle runs, what the AI analyses with
 * the progress and the game; after the one asked here, berhasil / gagal until closed. onlyMine: a page that shows only
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
  const { s } = useOrch()
  const running = s.cycleId === cycle.id && s.running
  const done = s.last?.cycleId === cycle.id
  const title = cycle.number ? `Analisis ulang ${cycle.label} · siklus #${cycle.number.toLocaleString('id-ID')}` : `Analisis ulang ${cycle.label}`
  return (
    <>
      <SheetHead icon="spark" title={title} sub={running ? 'Halaman ini boleh ditutup — analisis tetap berjalan dan hasilnya tersimpan' : done ? 'Selesai' : 'Menunggu giliran — siklus lain sedang berjalan'} onClose={closeSheet} />
      <div className="sec an-sheet">
        <AnalysisLive cycle={cycle} />
      </div>
      <div className="ft"><button className="btn quiet" onClick={closeSheet}>Tutup</button><span className="spacer" /><span className="pol"><Icon name="lock" />AI hanya mengusulkan · keputusan tetap di aplikasi</span></div>
    </>
  )
}
