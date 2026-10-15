import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MessageView } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { useThreads } from '../../app/queries'
import { displayTitle, matchThread } from './inbox'
import { mediaCaption, mediaName } from './bubble'

/** At most this many conversations per forward, like WhatsApp's limit on forwarding. */
export const MAX_FORWARD = 5

/** Teruskan: sends a bubble on to other conversations (WhatsApp "Teruskan"). Each target is its own reply — a human
 * decision through the outbox from that conversation's number; a photo or document goes with its caption. */
export function ForwardSheet({ m, from }: { m: MessageView; from: string }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: threads, isLoading } = useThreads('all', '')
  const [q, setQ] = useState('')
  const [picked, setPicked] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const list = useMemo(() => (threads ?? []).filter((t) => t.id !== from && matchThread(t, q)).slice(0, 60), [threads, from, q])
  const toggle = (id: string) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : p.length < MAX_FORWARD ? [...p, id] : p))
  const caption = mediaCaption(m.body, m.media)
  const preview = m.media ? [mediaName(m.media), caption].filter(Boolean).join(' · ') : caption
  const send = async () => {
    setBusy(true)
    setError('')
    let done = 0
    for (const id of picked) {
      try {
        await api.post(`/chat/threads/${id}/messages`, { forward: { thread_id: from, message_id: m.id } })
        done++
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Gagal meneruskan')
        break
      }
    }
    qc.invalidateQueries({ queryKey: ['chat'] })
    setBusy(false)
    if (done === picked.length) {
      toast(done === 1 ? 'Pesan diteruskan' : `Pesan diteruskan ke ${done} chat`)
      closeSheet()
    } else {
      setPicked(picked.slice(done)) // the ones not sent yet stay picked to try again
    }
  }
  return (
    <>
      <SheetHead icon="send" title="Teruskan pesan" sub={`Pilih hingga ${MAX_FORWARD} chat · dikirim dari nomor masing-masing chat setelah Anda menekan Teruskan`} onClose={closeSheet} />
      <div className="sec fwd">
        <div className="fwd-msg"><Icon name="chat" /><span>{preview || 'Pesan'}</span></div>
        <div className="ibl-search fwd-search"><Icon name="search" /><input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Cari chat tujuan" aria-label="Cari chat tujuan" autoFocus /></div>
        <div className="fwd-list" role="listbox" aria-multiselectable="true">
          {isLoading && <div className="chat-none">Memuat chat…</div>}
          {!isLoading && list.length === 0 && <div className="chat-none">{q ? `Tidak ada chat yang cocok dengan "${q}"` : 'Belum ada chat lain'}</div>}
          {list.map((t) => {
            const on = picked.includes(t.id)
            return (
              <button key={t.id} type="button" role="option" aria-selected={on} className={`fwd-row${on ? ' is-on' : ''}`} disabled={!on && picked.length >= MAX_FORWARD} onClick={() => toggle(t.id)}>
                <span className="fwd-check">{on && <Icon name="check" />}</span>
                <span className="fwd-who"><b>{displayTitle(t)}</b><small>{[t.subtitle, t.account_label && `via ${t.account_label}`].filter(Boolean).join(' · ')}</small></span>
              </button>
            )
          })}
        </div>
        {error && <small className="fwd-err">{error}</small>}
      </div>
      <div className="ft">
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
        <span className="spacer" />
        <button className="btn primary" disabled={!picked.length || busy} onClick={send}><Icon name="send" />{busy ? 'Meneruskan…' : `Teruskan${picked.length ? ` (${picked.length})` : ''}`}</button>
      </div>
    </>
  )
}
