import type { ReplySuggestion } from '../../api/types'
import { Icon } from '../../components/Icon'
import { hhmm, shortDate } from '../../lib/format'
import { ACTIONS } from './aitrigger'

/** An AI Trigger card under a customer's bubble (ADR 0038, 0041): the data the message asks about and a draft the
 * sales can use — it goes into the box to edit; the sales sends it. Also the result of "Coba kalimat". */
export function TriggerCard({ s, asOf, onUse, onAI, aiBusy }: {
  s: ReplySuggestion; asOf?: string | null; onUse?: (text: string) => void; onAI?: () => void; aiBusy?: boolean
}) {
  const info = ACTIONS[s.action] ?? ACTIONS.stock
  const stock = s.action === 'stock'
  return (
    <div className={`sug tone-${s.tone}`}>
      <div className="sug-h">
        <Icon name={info.icon} />
        <b>{s.headline || s.rule}</b>
        {s.product && <span>{s.product}</span>}
        <span className="sug-kind">{info.label}</span>
      </div>
      {stock && s.others.length > 0 && <div className="sug-o">Cabang lain: {s.others.slice(0, 4).map((o) => `${o.branch} ${o.qty.toLocaleString('id-ID')}`).join(' · ')}</div>}
      {s.facts.length > 0 && (
        <dl className="sug-f">
          {s.facts.map((f) => <div key={f.label}><dt>{f.label}</dt><dd>{f.value}</dd></div>)}
        </dl>
      )}
      {s.candidates && s.candidates.length > 0 && <div className="sug-o">Mungkin: {s.candidates.join(' · ')}</div>}
      {s.note && <div className="sug-n">{s.note}</div>}
      {s.draft && (
        <div className={`sug-d ${s.ai ? 'ai' : ''}`}>
          <span>{s.ai && <em className="sug-ai"><Icon name="spark" />AI</em>}“{s.draft}”</span>
          {onUse && <button type="button" className="btn ghost" onClick={() => onUse(s.draft!)}><Icon name="edit" />Pakai jawaban</button>}
          {onAI && !s.ai && !s.ai_pending && <button type="button" className="btn quiet" disabled={aiBusy} onClick={onAI}><Icon name="spark" />{aiBusy ? 'AI menulis…' : 'Tulis dengan AI'}</button>}
        </div>
      )}
      {s.ai_pending && !s.ai && <div className="sug-n sug-wait"><Icon name="spark" />AI sedang menulis jawaban…</div>}
      <small>
        Trigger “{s.rule}” · kata “{s.keyword}”{stock && asOf ? ` · stok sinkron ${shortDate(asOf)} ${hhmm(asOf)}` : ''}
        {s.ai && s.ai_model ? ` · ditulis ${s.ai_model}` : ''} · dikirim hanya bila Anda menekan kirim
      </small>
    </div>
  )
}
