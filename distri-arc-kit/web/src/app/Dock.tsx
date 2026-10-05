import { useState } from 'react'
import { useLocation } from 'react-router'
import { Icon } from '../components/Icon'
import { PIPELINE_STAGES, type ScreenKey } from '../lib/i18n/id'
import { chipState, progress } from './cycle'
import { useOrch, useOrchStatus } from './orch'

/** Pipeline chips (mockup pipeHtml compact): done / run per stage. */
export function PipeChips() {
  const { s } = useOrch()
  const st = useOrchStatus()
  return (
    <>
      {PIPELINE_STAGES.map((n, i) => (
        <span key={n} style={{ display: 'contents' }}>
          {i > 0 && <span className="pl" />}
          <span className={`ps ${chipState(s, i, st.hasCycle)}`} title={n}><i />{n}</span>
        </span>
      ))}
    </>
  )
}

/** Scope of "Analisis ulang layar ini" for a screen (04-orchestrator › Scope semantics). */
export function screenScope(screen: ScreenKey, path: string): string {
  switch (screen) {
    case 'orbit':
      return 'screen:orbit'
    case 'kuad':
      return 'screen:segmen'
    case 'net':
      return 'screen:relasi'
    case 'stock':
      return 'screen:stock'
    case 'ar':
      return 'screen:credit'
    case 'dealer': {
      const id = path.split('/')[2]
      return id ? 'dealer:' + id : 'screen:dealer'
    }
  }
  return 'all'
}

// Floating Orchestrator dock on every screen except Pusat kendali, Orchestrator and Chat.
export function Dock({ screen }: { screen: ScreenKey }) {
  const [open, setOpen] = useState(false)
  const { s, reanalyze } = useOrch()
  const st = useOrchStatus()
  const { pathname } = useLocation()
  const scope = screenScope(screen, pathname)
  return (
    <div className={`dock ${open ? 'open' : ''} ${st.running ? 'running' : ''}`}>
      <button className="dk-main" onClick={() => setOpen(!open)} aria-label="Orchestrator">
        <span className="dk-dot" />
        <span className="dk-txt">
          <b>Orchestrator</b>
          <span>{st.running ? `menganalisis ${st.scope} · ${st.stage}` : st.run ? `siap · siklus #${st.run.toLocaleString('id-ID')} · ${st.last}` : 'siap · belum ada siklus'}</span>
        </span>
      </button>
      <div className="dk-body">
        <div className="dk-pipe"><PipeChips /></div>
        <div className="dk-track"><i style={{ width: `${progress(s)}%` }} /></div>
        <div className="dk-acts">
          <button className="btn primary" disabled={st.running} onClick={() => reanalyze(scope)}><Icon name="refresh" />Analisis ulang layar ini</button>
          <button className="btn ghost" disabled={st.running} onClick={() => reanalyze('all')}>Semua</button>
          <button className="btn quiet" disabled={st.running} onClick={() => reanalyze('all', 'mcp')}><Icon name="plug" />Lewat MCP</button>
        </div>
      </div>
    </div>
  )
}
