import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { NextAction, Proposal } from '../api/types'
import { Icon } from './Icon'
import { SheetHead, useFeedback } from './feedback'
import { hhmm, shortDate } from '../lib/format'
import { PENDING_ORCH } from '../lib/i18n/id'
import { useProposal } from '../app/queries'

const REASONS: [string, string][] = [
  ['tidak_tepat_waktu', 'Tidak tepat waktu'], ['salah_dealer', 'Salah dealer / kontak'], ['sudah_dilakukan', 'Sudah dilakukan'],
  ['tidak_sesuai_kebijakan', 'Tidak sesuai kebijakan'], ['konteks_kurang', 'Konteks agen kurang'],
]
const SIG_ICON: Record<string, string> = { wa: 'chat', wa_group: 'people', so: 'doc', invoice: 'doc', payment: 'cash', stock: 'box', manual: 'form' }

export interface DecideBody { decision: 'approve' | 'edit' | 'reject' | 'option'; option?: string; reason?: string; reason_text?: string; preview?: string }

/** Decision mutation shared by the sheet and the queue: toast, then every dependent view refreshes. */
export function useDecide() {
  const qc = useQueryClient()
  const { toast, closeSheet } = useFeedback()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: DecideBody }) => api.post<{ status: string; result: string }>(`/proposals/${id}/decide`, body),
    onSuccess: (r) => {
      toast(r.result)
      closeSheet()
      for (const k of ['proposals', 'dealers', 'dealer', 'orbit', 'segmen', 'brief', 'chat', 'plan']) qc.invalidateQueries({ queryKey: [k] })
    },
    onError: (e: Error) => toast(e.message),
  })
}

/** The ActionSheet (mockup openSheet): provenance, why now, what is prepared, what happens after approval. */
export function ProposalSheet({ id }: { id: string }) {
  const { data: p } = useProposal(id)
  const { closeSheet } = useFeedback()
  const nav = useNavigate()
  const decide = useDecide()
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const [reasonText, setReasonText] = useState('')
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  if (!p) return <div className="sec"><p>Memuat…</p></div>
  const open = p.status === 'proposed'
  const go = (s: { kind: string }) => {
    closeSheet()
    if (s.kind === 'wa' || s.kind === 'wa_group') nav('/chat')
    else if (p.dealer_slug) nav('/dealer/' + p.dealer_slug + '#sec-tl')
  }
  return (
    <>
      <SheetHead icon={p.icon ?? 'spark'} title={p.title} sub={`${p.dealer_name ?? (typeof p.payload?.name === 'string' && !p.dealer_ids ? `Nomor baru · ${p.payload.name}` : 'Beberapa dealer')} · tenggat ${p.due_label ?? '—'}`} onClose={closeSheet} />
      <div className="prov-line">
        <span className="prov"><span className="ai" style={{ fontSize: 0 }} />{p.agent}</span>
        <span className="prov"><Icon name="doc" />dianalisis dari WhatsApp, SO, stok, dan pembayaran dealer ini</span>
        <span className="prov">confidence {p.confidence.toFixed(2)}</span>
        <span className="prov">{p.autonomy === 'auto' ? 'otonom · dalam batas kebijakan' : 'butuh approve'}</span>
      </div>
      {(p.signals ?? []).length > 0 && (
        <div className="sec">
          <h4>Sumber</h4>
          <div className="chips">
            {(p.signals ?? []).map((s) => (
              <button key={s.signal_id} className="chip" onClick={() => go(s)} title={s.text}>
                <Icon name={SIG_ICON[s.kind] ?? 'chat'} style={{ width: 12, height: 12, marginRight: 4, verticalAlign: '-2px' }} />
                {shortDate(s.at)} · {s.who || s.kind} · {s.text.length > 48 ? s.text.slice(0, 48) + '…' : s.text}
              </button>
            ))}
          </div>
        </div>
      )}
      <div className="sec"><h4><span className="ai" />Kenapa sekarang</h4><p>{p.why}</p></div>
      {p.prep && (
        <div className="sec">
          <h4>Yang sudah disiapkan</h4>
          <p>{p.prep}</p>
          {p.preview && !editing && <div className="prev">{p.preview}</div>}
          {editing && <textarea className="prev" style={{ width: '100%', minHeight: 96, font: 'inherit', fontSize: 13, color: 'var(--text)' }} value={draft} onChange={(e) => setDraft(e.target.value)} />}
        </div>
      )}
      {p.steps.length > 0 && (
        <div className="sec"><h4>Setelah Anda setujui</h4><ul className="steps">{p.steps.map((x) => <li key={x}><Icon name="check" /><span>{x}</span></li>)}</ul></div>
      )}
      {!open && (
        <div className="sec"><h4>Keputusan</h4><p>{statusText(p)}</p></div>
      )}
      {rejecting && (
        <div className="rej">
          <h4>Kenapa saran ini tidak tepat?</h4>
          <div className="chips">{REASONS.map(([k, l]) => <button key={k} className={`chip ${reason === k ? 'is-active' : ''}`} onClick={() => setReason(k)}>{l}</button>)}</div>
          <input value={reasonText} onChange={(e) => setReasonText(e.target.value)} placeholder="Tambahkan konteks (opsional) — ini yang dipelajari agen" />
          <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
            <button className="btn danger" disabled={!reason || decide.isPending} onClick={() => decide.mutate({ id: p.id, body: { decision: 'reject', reason, reason_text: reasonText } })}><Icon name="x" />Tolak saran ini</button>
            <button className="btn quiet" onClick={() => setRejecting(false)}>Batal</button>
          </div>
        </div>
      )}
      <div className="ft">
        {open && !editing && <button className="btn primary" disabled={decide.isPending} onClick={() => decide.mutate({ id: p.id, body: { decision: 'approve' } })}><Icon name="check" />Setujui &amp; jalankan</button>}
        {open && editing && <button className="btn primary" disabled={decide.isPending || !draft.trim()} onClick={() => decide.mutate({ id: p.id, body: { decision: 'edit', preview: draft } })}><Icon name="check" />Setujui hasil edit</button>}
        {open && p.preview && <button className="btn ghost" onClick={() => { setEditing(!editing); setDraft(p.preview ?? '') }}><Icon name="edit" />{editing ? 'Batal edit' : 'Edit dulu'}</button>}
        {open && <button className="btn ghost" style={{ color: 'var(--bad)' }} onClick={() => setRejecting(true)}><Icon name="x" />Tolak</button>}
        <button className="btn quiet" onClick={closeSheet}>Nanti</button>
        <span className="spacer" />
        <span className="pol"><Icon name="lock" />Tidak ada yang terkirim ke dealer tanpa langkah ini</span>
      </div>
    </>
  )
}

function statusText(p: Proposal) {
  const by = p.decided_by_name ? ` oleh ${p.decided_by_name}` : ''
  const at = p.decided_at ? ` · ${hhmm(p.decided_at)}` : ''
  switch (p.status) {
    case 'executed': return `Dijalankan${by}${at}${p.chosen_option && p.chosen_option !== 'approve' ? ` · opsi ${p.chosen_option}` : ''}`
    case 'approved': case 'edited': return p.autonomy === 'auto' && !p.decided_by_name ? `Otonom dalam batas kebijakan${at}` : `Disetujui${by}${at} · menunggu kirim`
    case 'rejected': return `Ditolak${by}${at} · ${p.decision_reason ?? ''}`
    case 'suppressed': return 'Ditahan kalibrasi: saran serupa ditolak dalam 14 hari terakhir'
    default: return 'Kedaluwarsa'
  }
}

/** Action button of a list row (mockup actBtn): opens the proposal, or shows how it was decided. */
export function ActBtn({ next, small, label, icon, ghost }: { next?: NextAction | null; small?: boolean; label?: string; icon?: string; ghost?: boolean }) {
  const { openSheet, toast } = useFeedback()
  const style = small ? { height: 28, fontSize: 12 } : undefined
  if (!next) {
    if (!label) return null
    return <button className="btn primary" style={style} onClick={() => toast(PENDING_ORCH)}><Icon name={icon ?? 'check'} />{label}</button>
  }
  if (next.status === 'executed' || ((next.status === 'approved' || next.status === 'edited') && next.autonomy === 'auto'))
    return <span className="pill good"><Icon name="check" />Dijalankan{next.executed_at || next.decided_at ? ` · ${hhmm((next.executed_at ?? next.decided_at)!)}` : ""}</span>
  if (next.status === 'approved' || next.status === 'edited') return <span className="pill accent"><Icon name="send" />Disetujui · mengirim</span>
  if (next.status === 'rejected') return <span className="pill bad"><Icon name="x" />Ditolak</span>
  if (next.status !== 'proposed') return null
  if (next.wait_for) return <span className="pill neutral"><Icon name="cash" />setelah {next.wait_for.replace(/^payment:/, '')} dibayar</span>
  return (
    <button className={`btn ${ghost ? 'ghost' : 'primary'}`} style={style} onClick={() => openSheet(<ProposalSheet id={next.id} />)}>
      <Icon name={next.icon || 'check'} />
      {next.button || 'Lihat'}
    </button>
  )
}
