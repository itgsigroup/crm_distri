import { useEffect, useRef, type MouseEvent as ReactMouseEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { ThreadView } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { displayTitle, unreplied } from './inbox'
import { LabelSheet } from './Labels'

/** "Ekspor chat" (ADR 0061): the browser saves "Chat WhatsApp dengan <nama>.txt" — WhatsApp's own export text. */
export function exportChat(id: string) {
  const a = document.createElement('a')
  a.href = `/api/chat/threads/${id}/export`
  a.download = ''
  document.body.appendChild(a)
  a.click()
  a.remove()
}

/** The menu of one chat in the list (⌄ or right click), like WhatsApp: pin to the top, label, mark unread / read,
 * mark unreplied / replied, export the chat. */
export function ChatRowMenu({ t, open, setOpen, onLeave }: { t: ThreadView; open: boolean; setOpen: (v: boolean) => void; onLeave: () => void }) {
  const qc = useQueryClient()
  const { toast, openSheet } = useFeedback()
  const ref = useRef<HTMLSpanElement>(null)
  useEffect(() => {
    if (!open) return
    const out = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false) }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.preventDefault(); setOpen(false) } }
    document.addEventListener('mousedown', out)
    document.addEventListener('keydown', esc)
    return () => { document.removeEventListener('mousedown', out); document.removeEventListener('keydown', esc) }
  }, [open, setOpen])
  const run = (p: Promise<unknown>, msg: string) => p.then(() => {
    qc.invalidateQueries({ queryKey: ['chat'] })
    qc.invalidateQueries({ queryKey: ['wa'] })
    toast(msg)
  }, (e: Error) => toast(e.message))
  const act = (f: () => void) => (e: ReactMouseEvent) => { e.stopPropagation(); setOpen(false); f() }
  const pinned = !!t.pinned_at
  const person = t.kind !== 'group' && t.contact_type !== 'supplier'
  const waiting = unreplied(t)
  return (
    <span className={`ibr-menu${open ? ' open' : ''}`} ref={ref}>
      <button type="button" className="ibr-menu-btn" aria-label="Menu chat" aria-haspopup="menu" aria-expanded={open}
        onClick={(e) => { e.stopPropagation(); setOpen(!open) }}><Icon name="chev" /></button>
      {open && (
        <span className="bub-menu-list ibr-menu-list" role="menu">
          <button type="button" role="menuitem" onClick={act(() => run(api.put(`/chat/threads/${t.id}/pin-chat`, { pinned: !pinned }), pinned ? 'Sematan chat dilepas' : 'Chat disematkan di atas daftar Anda'))}>
            <Icon name="pin" />{pinned ? 'Lepas sematan chat' : 'Sematkan chat'}</button>
          <button type="button" role="menuitem" onClick={act(() => openSheet(<LabelSheet title="Label chat" sub={displayTitle(t)} selected={t.labels ?? []}
            save={(ids) => api.put(`/chat/threads/${t.id}/labels`, { label_ids: ids })} />))}><Icon name="tag" />Label chat</button>
          {t.unread > 0
            ? <button type="button" role="menuitem" onClick={act(() => run(api.post(`/chat/threads/${t.id}/read`), 'Ditandai sudah dibaca'))}><Icon name="check" />Tandai sudah dibaca</button>
            : <button type="button" role="menuitem" onClick={act(() => { onLeave(); run(api.post(`/chat/threads/${t.id}/unread`), 'Ditandai belum dibaca') })}><Icon name="chat" />Tandai belum dibaca</button>}
          {person && (waiting
            ? <button type="button" role="menuitem" onClick={act(() => run(api.put(`/chat/threads/${t.id}/reply-state`, { state: 'replied' }), 'Ditandai sudah dibalas — keluar dari "Belum dibalas"'))}><Icon name="check" />Tandai sudah dibalas</button>
            : <button type="button" role="menuitem" onClick={act(() => run(api.put(`/chat/threads/${t.id}/reply-state`, { state: 'unreplied' }), 'Ditandai belum dibalas sampai ada balasan terkirim'))}><Icon name="send" />Tandai belum dibalas</button>)}
          <button type="button" role="menuitem" onClick={act(() => { exportChat(t.id); toast('Mengekspor chat · format teks WhatsApp, tanpa media') })}><Icon name="doc" />Ekspor chat</button>
        </span>
      )}
    </span>
  )
}
