import { useState } from 'react'
import { useNavigate } from 'react-router'
import type { DealerDetail, TimelineEntry } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { shortDate } from '../../lib/format'

const KIND: Record<string, string> = { wa: 'WhatsApp', wa_group: 'Grup', so: 'SO Odoo', invoice: 'Invoice', payment: 'Pembayaran', stock: 'Stok', manual: 'Catatan' }
const ICON: Record<string, string> = { wa: 'chat', wa_group: 'people', so: 'doc', invoice: 'doc', payment: 'cash', stock: 'box', manual: 'form' }

/** Sources of one memo sentence (click). */
function SourcesSheet({ text, sources, slug }: { text: string; sources: TimelineEntry[]; slug: string }) {
  const { closeSheet } = useFeedback()
  const nav = useNavigate()
  return (
    <>
      <SheetHead icon="spark" title="Sumber klaim" sub={text} onClose={closeSheet} />
      <div className="sec">
        <ul className="ext">
          {sources.map((s) => (
            <li key={s.signal_id}>
              <Icon name={ICON[s.kind] ?? 'doc'} />
              <span>
                <b>{KIND[s.kind] ?? s.kind}</b> · {shortDate(s.at)}{s.who ? ` · ${s.who}` : ''}<br />{s.text}
                {s.conclusion && <><br /><span style={{ color: 'var(--text-3)', fontSize: 11.5 }}>{s.conclusion}</span></>}
              </span>
            </li>
          ))}
        </ul>
      </div>
      <div className="ft">
        {sources.some((s) => s.kind === 'wa') && <button className="btn ghost" onClick={() => { closeSheet(); nav('/chat') }}><Icon name="chat" />Buka chat</button>}
        <button className="btn ghost" onClick={() => { closeSheet(); nav('/dealer/' + slug + '#sec-tl') }}><Icon name="doc" />Ke timeline</button>
        <button className="btn quiet" onClick={closeSheet}>Tutup</button>
      </div>
    </>
  )
}

/** Memori dealer: every sentence shows its sources on hover and opens them on click. */
export function MemoText({ d }: { d: DealerDetail }) {
  const { openSheet } = useFeedback()
  const [hover, setHover] = useState<{ i: number; x: number; y: number } | null>(null)
  const byId = new Map((d.memo_signals ?? []).map((s) => [s.signal_id, s]))
  const sentences = d.memo_sentences ?? []
  if (sentences.length === 0) return <p className="memo">{d.memo}</p>
  const srcOf = (i: number) => sentences[i].signal_ids.map((id) => byId.get(id)).filter((s): s is TimelineEntry => !!s)
  return (
    <div className="memo-wrap" onMouseLeave={() => setHover(null)}>
      <p className="memo">
        {sentences.map((s, i) => (
          <span key={i}>
            {i > 0 && ' '}
            <span
              className={`ms ${hover?.i === i ? 'on' : ''}`}
              onMouseMove={(e) => {
                const r = (e.currentTarget.closest('.memo-wrap') as HTMLElement).getBoundingClientRect()
                setHover({ i, x: Math.min(e.clientX - r.left + 12, r.width - 330), y: e.clientY - r.top + 16 })
              }}
              onClick={() => openSheet(<SourcesSheet text={s.text} sources={srcOf(i)} slug={d.id} />)}
            >{s.text}</span>
          </span>
        ))}
      </p>
      {hover && (
        <div className="tip show" style={{ left: Math.max(0, hover.x), top: hover.y }}>
          <b>{srcOf(hover.i).length} sumber</b>
          {srcOf(hover.i).map((s) => <div className="r" key={s.signal_id}><span>{KIND[s.kind] ?? s.kind} · {shortDate(s.at)}</span><span>{s.text.length > 60 ? s.text.slice(0, 60) + '…' : s.text}</span></div>)}
        </div>
      )}
    </div>
  )
}
