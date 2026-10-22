import { useQuery } from '@tanstack/react-query'
import { Icon } from '../../components/Icon'
import { PlanText } from '../control/Plan'
import { askQuery } from '../../app/AskSheet'
import { hhmm } from '../../lib/format'

/** Hasil AI Orchestrator on Cek stok (Sam 22 Oct): the question asked in the Orchestrator box and its answer, above
 * the tables it filtered (branch, search, Analisa stok view). × closes it and lifts the filters. */
export function AskResult({ q, onClose }: { q: string; onClose: () => void }) {
  const { data: a, isLoading, error } = useQuery(askQuery(q))
  return (
    <div className="card ask-res" role="status" aria-live="polite">
      <div className="ask-res-h">
        <span className="ask-res-tag"><span className="ai" style={{ fontSize: 0 }} />Hasil AI · Orchestrator</span>
        <b>“{q}”</b>
        <button className="close" onClick={onClose} aria-label="Tutup hasil dan lepas filter" title="Tutup hasil dan lepas filter"><Icon name="x" /></button>
      </div>
      <p>{error ? (error as Error).message : isLoading || !a ? 'Orchestrator membaca data stok…' : <PlanText text={a.text} />}</p>
      {a && (
        <small>
          {a.via === 'llm' ? 'Ditulis AI dari angka sistem' : 'Angka dihitung sistem dari data stok'} · {hhmm(a.at)} WIB
          {a.show?.branch ? ` · tabel difilter cabang ${a.show.branch}` : ''}{a.show?.search ? ` · cari “${a.show.search}”` : ''}
        </small>
      )}
    </div>
  )
}
