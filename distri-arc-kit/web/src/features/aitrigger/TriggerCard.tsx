import type { ReplySuggestion } from '../../api/types'
import { Icon } from '../../components/Icon'
import { hhmm, shortDate } from '../../lib/format'
import { ACTIONS } from './aitrigger'

/** The AI a draft came from, by its model ("gpt-4.1-mini" → ChatGPT). */
export const aiName = (model?: string) => (!model ? 'AI' : /^(gpt|o\d)/i.test(model) ? 'ChatGPT' : /claude/i.test(model) ? 'Claude' : 'AI')

export type DraftAI = 'openai' | 'claude'

/** An AI Trigger card under a customer's bubble (ADR 0038, 0041): the data the message asks about and a draft the
 * sales can use — it goes into the box to edit; the sales sends it. Also the result of "Coba kalimat". */
export function TriggerCard({ s, asOf, onUse, onAI, aiBusy }: {
  s: ReplySuggestion; asOf?: string | null; onUse?: (text: string) => void
  /** write (or rewrite) the draft with ChatGPT or Claude (Sam 19 Oct) */
  onAI?: (ai: DraftAI) => void; aiBusy?: DraftAI | null
}) {
  const info = ACTIONS[s.action] ?? ACTIONS.stock
  const stock = s.action === 'stock'
  return (
    <div className={`sug tone-${s.tone}`}>
      <div className="sug-tag"><Icon name="spark" />AI Trigger · {info.label}<span>· hanya terlihat tim, bukan pesan</span></div>
      <div className="sug-h">
        <Icon name={info.icon} />
        <b>{s.headline || s.rule}</b>
        {s.product && <span>{s.product}</span>}
      </div>
      {s.from_history && <div className="sug-hist">Produk dari riwayat chat: <q>{s.from_history}</q></div>}
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
          <span>{s.ai && <em className="sug-ai"><Icon name="spark" />{aiName(s.ai_model)}</em>}“{s.draft}”</span>
          {onUse && <button type="button" className="btn ghost" onClick={() => onUse(s.draft!)}><Icon name="edit" />Pakai jawaban</button>}
          {onAI && !s.ai_pending && (
            <span className="sug-ais" role="group" aria-label={s.ai ? 'Tulis ulang dengan' : 'Tulis dengan'}>
              <small>{s.ai ? 'Tulis ulang:' : 'Tulis dengan:'}</small>
              {([['openai', 'ChatGPT'], ['claude', 'Claude']] as const).map(([k, l]) => (
                <button key={k} type="button" className={`btn quiet sug-ai-btn ai-${k}`} disabled={!!aiBusy} onClick={() => onAI(k)}>
                  <Icon name="spark" />{aiBusy === k ? `${l} menulis…` : l}
                </button>
              ))}
            </span>
          )}
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
