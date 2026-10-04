import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { Action, DecisionRequest, DecisionResponse } from '../api/types'
import { useUI } from '../state/ui'
import { Icon } from './ui'

const REASONS = ['Tidak tepat waktu', 'Salah kontak / jalur', 'Sudah dilakukan', 'Tidak sesuai kebijakan', 'Konteks agen kurang']

// Global action sheet: provenance line, Kenapa sekarang, Yang disiapkan (+preview),
// Setelah disetujui, and the human decision buttons. Nothing reaches a customer
// without one of these buttons being pressed.
export function ActionSheet() {
  const { sheet, closeSheet, toast, refresh } = useUI()
  const [action, setAction] = useState<Action | null>(null)
  const [rejectOpen, setRejectOpen] = useState(false)
  const [reason, setReason] = useState<string | null>(null)
  const [note, setNote] = useState('')
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)

  // Reset the sheet's local state when a different action opens (during render, not in an effect).
  const [shown, setShown] = useState<Action | string | null>(null)
  if (sheet !== shown) {
    setShown(sheet)
    setRejectOpen(false)
    setReason(null)
    setNote('')
    setEditing(false)
    if (sheet && typeof sheet !== 'string') {
      setAction(sheet)
      setDraft(sheet.preview)
    } else if (typeof sheet === 'string') {
      setAction(null)
    }
  }
  useEffect(() => {
    if (typeof sheet !== 'string') return
    api.get<Action>('/api/actions/' + encodeURIComponent(sheet)).then(a => {
      setAction(a)
      setDraft(a.preview)
    }).catch(e => toast('Gagal membuka saran: ' + e.message))
  }, [sheet, toast])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && closeSheet()
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [closeSheet])

  const decide = async (req: DecisionRequest) => {
    if (!action || busy) return
    setBusy(true)
    try {
      const r = await api.post<DecisionResponse>('/api/actions/' + encodeURIComponent(action.id) + '/decision', req)
      closeSheet()
      toast(r.toast)
      refresh()
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const show = !!sheet
  const a = action
  const decided = a && a.status !== 'proposed' && a.status !== 'snoozed'
  return (
    <>
      <div className={'sheet-bg' + (show ? ' show' : '')} onClick={closeSheet} />
      <div className={'sheet' + (show ? ' show' : '')} role="dialog" aria-modal="true" aria-labelledby="sheet-title">
        {a && (
          <>
            <div className="sh">
              <span className="ni"><Icon n={a.icon} /></span>
              <div>
                <h3 id="sheet-title">{a.title}</h3>
                <small>{a.account_name}{a.due_label ? ' · tenggat ' + a.due_label : ''}</small>
              </div>
              <button className="close" onClick={closeSheet} aria-label="Tutup"><Icon n="i-x" /></button>
            </div>
            <div className="prov-line">
              <span className="prov"><span className="ai" style={{ fontSize: 0 }} />{a.agent}</span>
              <span className="prov"><Icon n="i-doc" />{a.provenance_line}</span>
              <span className="prov">confidence {a.confidence.toFixed(2)}</span>
              <span className="prov">model: {a.model}</span>
            </div>
            <div className="sec"><h4><span className="ai" />Kenapa sekarang</h4><p>{a.why}</p></div>
            <div className="sec">
              <h4>Yang sudah disiapkan ARC</h4>
              <p>{a.prep}</p>
              {editing ? (
                <textarea
                  className="prev"
                  style={{ width: '100%', minHeight: 140, font: 'inherit', fontSize: 13, color: 'var(--text)', resize: 'vertical' }}
                  value={draft}
                  onChange={e => setDraft(e.target.value)}
                  aria-label="Edit draf"
                />
              ) : a.preview ? <div className="prev">{a.preview}</div> : null}
            </div>
            <div className="sec">
              <h4>Setelah Anda setujui</h4>
              <ul className="steps">{a.steps.map((x, i) => <li key={i}><Icon n="i-check" /><span>{x}</span></li>)}</ul>
            </div>
            {rejectOpen && (
              <div className="rej" id="rej-box">
                <h4>Kenapa saran ini tidak tepat?</h4>
                <div className="chips">
                  {REASONS.map(r => (
                    <button key={r} className={'chip' + (reason === r ? ' is-active' : '')} onClick={() => setReason(r)}>{r}</button>
                  ))}
                </div>
                <input value={note} onChange={e => setNote(e.target.value)} placeholder="Tambahkan konteks (opsional) — ini yang dipelajari agen" />
                <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                  <button className="btn danger" disabled={busy} onClick={() => decide({ decision: 'reject', reason: reason || REASONS[0], note })}><Icon n="i-x" />Tolak saran ini</button>
                  <button className="btn quiet" onClick={() => setRejectOpen(false)}>Batal</button>
                </div>
              </div>
            )}
            <div className="ft">
              {decided ? (
                <span className={'pill ' + (a.status === 'rejected' ? 'bad' : 'good')}><Icon n={a.status === 'rejected' ? 'i-x' : 'i-check'} />{a.result_text || a.status}</span>
              ) : a.options.length > 0 ? (
                a.options.map(o => (
                  <button key={o.key} className={'btn ' + (o.primary ? 'primary' : 'ghost')} disabled={busy} onClick={() => decide({ decision: 'option', option: o.key })}>{o.label}</button>
                ))
              ) : editing ? (
                <>
                  <button className="btn primary" disabled={busy} onClick={() => decide({ decision: 'edit', preview: draft })}><Icon n="i-check" />Simpan &amp; jalankan</button>
                  <button className="btn quiet" onClick={() => { setEditing(false); setDraft(a.preview) }}>Batal edit</button>
                </>
              ) : (
                <>
                  <button className="btn primary" disabled={busy} onClick={() => decide({ decision: 'approve' })}><Icon n="i-check" />Setujui &amp; jalankan</button>
                  <button className="btn ghost" onClick={() => setEditing(true)}><Icon n="i-edit" />Edit dulu</button>
                  <button className="btn ghost" style={{ color: 'var(--bad)' }} onClick={() => setRejectOpen(true)}><Icon n="i-x" />Tolak</button>
                  <button className="btn quiet" disabled={busy} onClick={() => decide({ decision: 'snooze' })}>Nanti</button>
                </>
              )}
              {!decided && a.options.length > 0 && (
                <button className="btn ghost" style={{ color: 'var(--bad)' }} onClick={() => setRejectOpen(true)}><Icon n="i-x" />Tolak</button>
              )}
              <span className="spacer" />
              <span className="pol"><Icon n="i-lock" />Tidak ada yang terkirim ke pelanggan tanpa langkah ini</span>
            </div>
          </>
        )}
      </div>
    </>
  )
}

export function Toast() {
  const { toastMsg } = useUI()
  // Keep the last message so the text stays while the toast fades out.
  const [last, setLast] = useState('')
  if (toastMsg && toastMsg !== last) setLast(toastMsg)
  return (
    <div className={'toast' + (toastMsg ? ' show' : '')} role="status" aria-live="polite">
      <Icon n="i-check" />
      {toastMsg || last}
    </div>
  )
}
