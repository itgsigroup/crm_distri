import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { hhmm, shortDate } from '../../lib/format'
import { countLabel, marks, step, type ChatHit } from './findInChat'

/** Text with the searched words highlighted (WhatsApp's yellow). */
export function Marked({ text, q }: { text: string; q: string }) {
  return <>{marks(text, q).map((p, i) => (p.hit ? <mark key={i} className="cs-mark">{p.t}</mark> : p.t))}</>
}

/** Cari di dalam chat: a bar under the chat's header — the words, the results (newest first) with the text around
 * them, ▲ older / ▼ newer, and a jump to the bubble. */
export function ChatSearch({ thread, onJump, onQuery, onClose }: {
  thread: string; onJump: (h: ChatHit) => void; onQuery: (q: string) => void; onClose: () => void
}) {
  const [q, setQ] = useState('')
  const [dq, setDq] = useState('')
  const [cur, setCur] = useState(-1)
  const [list, setList] = useState(true)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const t = window.setTimeout(() => {
      setDq(q.trim().replace(/\s+/g, ' '))
      setCur(-1)
      setList(true)
    }, 250)
    return () => window.clearTimeout(t)
  }, [q])
  useEffect(() => { onQuery(dq.length >= 2 ? dq : '') }, [dq, onQuery])
  const { data, isFetching } = useQuery({
    queryKey: ['chat', 'search', thread, dq],
    queryFn: () => api.get<{ items: ChatHit[]; capped?: boolean }>(`/chat/threads/${thread}/search?q=${encodeURIComponent(dq)}`),
    enabled: dq.length >= 2,
    staleTime: 15_000,
  })
  const hits = dq.length >= 2 ? data?.items ?? [] : []
  const go = (i: number) => {
    if (i < 0 || i >= hits.length) return
    setCur(i)
    setList(false)
    onJump(hits[i])
  }
  const key = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') { e.preventDefault(); onClose(); return }
    if (e.key === 'Enter') { e.preventDefault(); go(step(cur, hits.length, e.shiftKey ? 'down' : 'up')) }
  }
  return (
    <div className="cs">
      <div className="cs-bar">
        <Icon name="search" />
        <input ref={input} autoFocus value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={key} onFocus={() => setList(true)}
          placeholder="Cari kata atau kalimat di chat ini" aria-label="Cari di chat ini" />
        {dq.length >= 2 && <span className="cs-count">{isFetching && !data ? 'Mencari…' : countLabel(cur, hits.length, !!data?.capped)}</span>}
        <button type="button" className="ibl-btn" disabled={!hits.length || cur >= hits.length - 1} onClick={() => go(step(cur, hits.length, 'up'))} title="Lebih lama (Enter)" aria-label="Hasil lebih lama"><Icon name="arrow" style={{ transform: 'rotate(-90deg)' }} /></button>
        <button type="button" className="ibl-btn" disabled={!hits.length || cur <= 0} onClick={() => go(step(cur, hits.length, 'down'))} title="Lebih baru (Shift+Enter)" aria-label="Hasil lebih baru"><Icon name="arrow" style={{ transform: 'rotate(90deg)' }} /></button>
        <button type="button" className="ibl-btn" onClick={onClose} title="Tutup (Esc)" aria-label="Tutup pencarian"><Icon name="x" /></button>
      </div>
      {list && hits.length > 0 && (
        <ul className="cs-list" role="listbox" aria-label="Hasil pencarian">
          {hits.map((h, i) => (
            <li key={h.id} role="option" aria-selected={i === cur}>
              <button type="button" className={i === cur ? 'on' : ''} onClick={() => go(i)}>
                <span className="cs-when">{shortDate(h.sent_at)} · {hhmm(h.sent_at)}<b>{h.direction === 'out' ? 'Anda' : h.from_name}</b></span>
                <span className="cs-snip"><Marked text={h.snippet} q={dq} /></span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
