import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Cycle } from '../api/types'
import { Icon } from '../components/Icon'
import { SheetHead, useFeedback } from '../components/feedback'
import { AnalysisResult, AnalysisRunning } from '../features/control/AnalysisPanel'
import { useOrch } from './orch'

/** How a cycle asked from the "Analisis ulang" button is analysed by Claude through MCP (POST /cycles → mcp). */
export interface CycleMCP { engine: 'claude_api' | 'claude_ai'; /** claude_api: "Claude API · Claude Opus 5.5" */ model?: string; prompt: string; claude_url: string; wait_sec: number; agents: string[]; /** the API second option (ADR 0040), "" when off */ fallback?: string }
export type QueuedCycle = Cycle & { mcp?: CycleMCP }

interface InputState { agent: string; submitted_at: string | null }

/** The pop-up of an analysis started with "Analisis ulang" (Sam 16 Oct: in place of the MCP steps): "AI sedang membaca
 * data…" with the game while it runs, then berhasil / gagal. Through Claude (claude.ai) it keeps one button to open
 * Claude with the task — Claude only works when someone sends it there. It announces the result itself (no alert). */
export function AnalysisSheet({ cycle }: { cycle: QueuedCycle }) {
  const { closeSheet } = useFeedback()
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
  const claude = m?.engine === 'claude_ai'
  const hint = m && (
    <div className="an-mcp">
      <span>
        {sent > 0 ? <>Claude sudah mengirim <b>{sent}/{m.agents.length}</b> agen.</>
          : claude ? <>Analisis lewat Claude: buka Claude lalu tekan Enter agar mulai membaca.{m.fallback ? <> Bila Claude tidak menjawab dalam {Math.round(m.wait_sec / 60)} menit, <b>{m.fallback}</b> melanjutkan di server.</> : null}</>
          : <><b>{m.model || 'API'}</b> membaca di server — tidak perlu membuka apa pun.</>}
      </span>
      {claude && sent === 0 && <a className="btn primary" href={m.claude_url} target="_blank" rel="noopener noreferrer"><Icon name="plug" />Buka Claude</a>}
    </div>
  )
  const title = cycle.number ? `Analisis ulang ${cycle.label} · siklus #${cycle.number.toLocaleString('id-ID')}` : `Analisis ulang ${cycle.label}`
  return (
    <>
      <SheetHead icon="spark" title={title} sub={running ? 'Halaman ini boleh ditutup — analisis tetap berjalan dan hasilnya tersimpan' : 'Selesai'} onClose={closeSheet} />
      <div className="sec an-sheet">
        {last ? <AnalysisResult last={last} /> : <AnalysisRunning extra={hint} focusGame={!claude} />}
      </div>
      <div className="ft"><button className="btn quiet" onClick={closeSheet}>Tutup</button><span className="spacer" /><span className="pol"><Icon name="lock" />AI hanya mengusulkan · keputusan tetap di aplikasi</span></div>
    </>
  )
}
