import { useState } from 'react'
import type { Proposal } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ProposalSheet, useDecide } from '../../components/actions'
import { useFeedback } from '../../components/feedback'

const TONE: Record<string, string> = { credit_release: 'var(--bad)', price_counter: 'var(--warn)', credit_limit: 'var(--good)', return: 'var(--indigo)' }

function resultOf(p: Proposal) {
  const o = p.options.find((x) => x.key === (p.chosen_option ?? 'approve'))
  if (p.status === 'rejected') return 'Ditolak · ' + (p.decision_reason ?? 'alasan dicatat untuk kalibrasi')
  if (p.status === 'expired') return o?.result ?? 'Ditunda'
  return o?.result ?? 'Diputuskan'
}

/** "Keputusan" (mockup renderQueue): high-stakes proposals with impact figures and alternative options. */
export function Queue({ items }: { items: Proposal[] }) {
  const [openId, setOpenId] = useState<string | null>(null)
  const decide = useDecide()
  const { openSheet } = useFeedback()
  const firstOpen = items.find((q) => q.status === 'proposed')?.id
  const isOpen = (q: Proposal) => (openId === null ? q.id === firstOpen : openId === q.id)
  const act = (q: Proposal, key: string) => {
    if (key === 'reject') openSheet(<ProposalSheet id={q.id} />)
    else decide.mutate({ id: q.id, body: key === 'approve' ? { decision: 'approve' } : { decision: 'option', option: key } })
  }
  if (items.length === 0)
    return (
      <div className="queue">
        <div className="q done" style={{ opacity: 1 }}>
          <div className="qi" style={{ color: 'var(--text-3)' }}><Icon name="check" /></div>
          <div><div className="qt"><b>Tidak ada keputusan hari ini</b></div><div className="qd">Saran di luar batas otonomi — rilis kredit, harga di bawah tier, retur, kenaikan limit — muncul di sini lengkap dengan alasan, dampak, dan opsi.</div></div>
        </div>
      </div>
    )
  return (
    <div className="queue">
      {items.map((q) => {
        const done = q.status !== 'proposed'
        const quick = q.options[0]
        return (
          <div key={q.id} className={`q ${isOpen(q) && !done ? 'open' : ''} ${done ? 'done' : ''}`}>
            <div className="qi" style={{ color: TONE[q.kind] ?? 'var(--accent)' }}><Icon name={q.icon ?? 'spark'} /></div>
            <div>
              <div className="q-head" onClick={(e) => { if (!(e.target as HTMLElement).closest('button')) setOpenId(isOpen(q) ? '' : q.id) }}>
                <div className="qt"><b>{queueTitle(q)}</b>{q.pills.map(([k, t]) => <span key={t} className={`pill ${k}`}>{t}</span>)}</div>
                {quick && !done && <div className="q-quick"><button className="btn primary" onClick={() => act(q, quick.key)}>{q.button ?? quick.label}</button></div>}
                <Icon name="chev" className="i chev" />
              </div>
              <div className="q-body">
                <div className="qd">{q.summary}</div>
                <div className="impact">{q.impact.map((m) => <div key={m.label}>{m.label}<b className="num" style={m.tone ? { color: `var(--${m.tone})` } : undefined}>{m.value}</b></div>)}</div>
                <div className="qw"><span className="ai" /><span>{q.why}</span></div>
                <div className="qa">
                  {q.options.map((o) => <button key={o.key} className={`btn ${o.style}`} disabled={decide.isPending} onClick={() => act(q, o.key)}>{o.label}</button>)}
                  <button className="btn quiet" onClick={() => openSheet(<ProposalSheet id={q.id} />)}>Detail</button>
                </div>
              </div>
              {done && <div className="qres" style={{ display: 'flex' }}><Icon name="check" /><span>{resultOf(q)}</span></div>}
            </div>
          </div>
        )
      })}
    </div>
  )
}

/** Queue titles follow the mockup ("Rilis barang kredit — PT Graha Sentosa"). */
function queueTitle(q: Proposal) {
  const d = q.dealer_name ?? ''
  switch (q.kind) {
    case 'credit_release': return `Rilis barang kredit — ${d}`
    case 'price_counter': {
      const pct = q.impact[1]?.label.replace('Jika ', '') ?? ''
      return `Harga khusus ${d} — ${pct} di bawah tier`
    }
    case 'credit_limit': return `Naikkan limit kredit ${d} → ${q.impact[1]?.value ?? ''}`
    case 'return': return q.title.replace(/^Setujui retur (.*?) (PT|CV|UD|Toko)? ?(.*) \(ganti unit\)$/, 'Retur $1 — ' + d)
    default: return q.title
  }
}
