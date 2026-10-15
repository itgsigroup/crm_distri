import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { ChatLabel, ChatPin } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { hhmm, shortDate } from '../../lib/format'
import { LABEL_COLORS, labelsOf, nextPin, pinLine } from './chatLabels'

export const useChatLabels = () => useQuery({
  queryKey: ['chat', 'labels'],
  queryFn: () => api.get<{ items: ChatLabel[]; can_manage: boolean }>('/chat/labels'),
  staleTime: 60_000,
})

/** The labels as small colored chips. */
export function LabelChips({ ids, className = '' }: { ids?: string[]; className?: string }) {
  const { data } = useChatLabels()
  const ls = labelsOf(ids, data?.items ?? [])
  if (!ls.length) return null
  return (
    <span className={`lbl-chips ${className}`}>
      {ls.map((l) => <span key={l.id} className="lbl-chip" style={{ ['--lbl' as string]: l.color }}><i />{l.name}</span>)}
    </span>
  )
}

/** Pick the labels of a chat or a message; a new label can be made here, an admin can delete one. */
export function LabelSheet({ title, sub, selected, save }: { title: string; sub: string; selected: string[]; save: (ids: string[]) => Promise<unknown> }) {
  const { data } = useChatLabels()
  const qc = useQueryClient()
  const { toast, closeSheet } = useFeedback()
  const [pick, setPick] = useState<string[]>(selected)
  const [name, setName] = useState('')
  const [color, setColor] = useState(LABEL_COLORS[0])
  const create = useMutation({
    mutationFn: () => api.post<ChatLabel>('/chat/labels', { name: name.trim(), color }),
    onSuccess: (l) => { setPick((p) => [...p, l.id]); setName(''); qc.invalidateQueries({ queryKey: ['chat', 'labels'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const del = useMutation({
    mutationFn: (l: ChatLabel) => api.del(`/chat/labels/${l.id}`),
    onSuccess: (_, l) => { setPick((p) => p.filter((x) => x !== l.id)); toast(`Label ${l.name} dihapus`); qc.invalidateQueries({ queryKey: ['chat'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const done = useMutation({
    mutationFn: () => save(pick),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['chat'] }); toast('Label disimpan'); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const toggle = (id: string) => setPick((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]))
  return (
    <>
      <SheetHead icon="tag" title={title} sub={sub} onClose={closeSheet} />
      <div className="sec lbl-sheet">
        <ul className="lbl-list">
          {(data?.items ?? []).map((l) => (
            <li key={l.id}>
              <label>
                <input type="checkbox" checked={pick.includes(l.id)} onChange={() => toggle(l.id)} />
                <span className="lbl-dot" style={{ background: l.color }} />
                <span className="lbl-name">{l.name}</span>
                <small>{l.chats} chat</small>
              </label>
              {data?.can_manage && <button type="button" className="ibl-btn" title={`Hapus label ${l.name}`} aria-label={`Hapus label ${l.name}`}
                onClick={() => { if (window.confirm(`Hapus label "${l.name}" dari semua chat dan pesan?`)) del.mutate(l) }}><Icon name="x" /></button>}
            </li>
          ))}
        </ul>
        <div className="lbl-new">
          <input value={name} onChange={(e) => setName(e.target.value)} maxLength={40} placeholder="Label baru, mis. Proyek RS" aria-label="Nama label baru"
            onKeyDown={(e) => { if (e.key === 'Enter' && name.trim()) { e.preventDefault(); create.mutate() } }} />
          <span className="lbl-colors" role="radiogroup" aria-label="Warna label">
            {LABEL_COLORS.map((c) => <button key={c} type="button" role="radio" aria-checked={color === c} aria-label={c} className={color === c ? 'on' : ''} style={{ background: c }} onClick={() => setColor(c)} />)}
          </span>
          <button type="button" className="btn ghost" disabled={!name.trim() || create.isPending} onClick={() => create.mutate()}>Tambah</button>
        </div>
      </div>
      <div className="ft">
        <button className="btn primary" disabled={done.isPending} onClick={() => done.mutate()}><Icon name="check" />Simpan</button>
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
      </div>
    </>
  )
}

/** The pinned messages above the chat: one at a time, a click goes to it and shows the next (like WhatsApp). */
export function PinnedBar({ pins, who, onJump, onUnpin }: { pins: ChatPin[]; who: string; onJump: (p: ChatPin) => void; onUnpin: (p: ChatPin) => void }) {
  const [cur, setCur] = useState(0)
  if (!pins.length) return null
  const i = cur < pins.length ? cur : 0
  const p = pins[i]
  return (
    <div className="pin-bar">
      {pins.length > 1 && <span className="pin-ticks" aria-hidden>{pins.map((x, k) => <i key={x.message_id} className={k === i ? 'on' : ''} />)}</span>}
      <button type="button" className="pin-main" onClick={() => { onJump(p); setCur(nextPin(i, pins.length)) }}
        title={`Disematkan ${p.pinned_by ? 'oleh ' + p.pinned_by + ' ' : ''}${shortDate(p.pinned_at)} ${hhmm(p.pinned_at)} — klik untuk ke pesan`}>
        <Icon name="pin" />
        <span><b>{p.direction === 'out' ? 'Anda' : p.from_name || who}{pins.length > 1 ? ` · ${i + 1}/${pins.length}` : ''}</b>{pinLine(p)}</span>
      </button>
      <button type="button" className="ibl-btn" onClick={() => onUnpin(p)} title="Lepas sematan" aria-label="Lepas sematan"><Icon name="x" /></button>
    </div>
  )
}
