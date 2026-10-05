import { useState } from 'react'
import { Icon } from '../components/Icon'
import { useFeedback } from '../components/feedback'
import { PENDING_ORCH, PIPELINE_STAGES, type ScreenKey } from '../lib/i18n/id'
import { useOrchStatus } from './orch'

export function PipeChips() {
  return (
    <>
      {PIPELINE_STAGES.map((n, i) => (
        <span key={n} style={{ display: 'contents' }}>
          {i > 0 && <span className="pl" />}
          <span className="ps" title={n}><i />{n}</span>
        </span>
      ))}
    </>
  )
}

// Floating Orchestrator dock on every screen except Pusat kendali, Orchestrator and Chat.
export function Dock({ screen }: { screen: ScreenKey }) {
  const [open, setOpen] = useState(false)
  const s = useOrchStatus()
  const { toast } = useFeedback()
  void screen
  return (
    <div className={`dock ${open ? 'open' : ''}`}>
      <button className="dk-main" onClick={() => setOpen(!open)} aria-label="Orchestrator">
        <span className="dk-dot" />
        <span className="dk-txt"><b>Orchestrator</b><span>siap · {s.run ? 'siklus #' + s.run.toLocaleString('id-ID') : 'belum ada siklus'}</span></span>
      </button>
      <div className="dk-body">
        <div className="dk-pipe"><PipeChips /></div>
        <div className="dk-track"><i style={{ width: '0%' }} /></div>
        <div className="dk-acts">
          <button className="btn primary" onClick={() => toast(PENDING_ORCH)}><Icon name="refresh" />Analisis ulang layar ini</button>
          <button className="btn ghost" onClick={() => toast(PENDING_ORCH)}>Semua</button>
          <button className="btn quiet" onClick={() => toast(PENDING_ORCH)}><Icon name="plug" />Lewat MCP</button>
        </div>
      </div>
    </div>
  )
}
