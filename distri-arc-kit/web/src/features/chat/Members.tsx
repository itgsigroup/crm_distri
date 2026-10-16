import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { fmtPhone } from './inbox'
import type { Member } from './mentions'

// Anggota grup, like WhatsApp's group info: the members (admins first) — a click opens the one-to-one chat with them.

export interface Members { items: Member[]; hidden: number }

/** A group's members, shared by the Anggota sheet and the composer's "@" list. */
export function useMembers(thread: string, enabled: boolean) {
  return useQuery({
    queryKey: ['chat', 'members', thread],
    queryFn: () => api.get<Members>(`/chat/threads/${thread}/members`),
    enabled,
    staleTime: 5 * 60_000,
  })
}

/** Opens (makes when needed) the one-to-one chat of the group thread's number with a member; resolves to its id. */
export function openMemberChat(thread: string, number: string): Promise<string> {
  return api.post<{ thread_id: string }>(`/chat/threads/${thread}/members/${number}/open`, {}).then((r) => r.thread_id)
}

export function MembersSheet({ thread, group, onOpen }: { thread: string; group: string; onOpen: (number: string) => Promise<void> }) {
  const { closeSheet } = useFeedback()
  const { data, isLoading } = useMembers(thread, true)
  const [q, setQ] = useState('')
  const [busy, setBusy] = useState('')
  const s = q.trim().toLocaleLowerCase('id-ID')
  const list = (data?.items ?? []).filter((m) => !s || m.name.toLocaleLowerCase('id-ID').includes(s) || m.number.includes(s.replace(/\D/g, '') || s))
  const open = (m: Member) => {
    setBusy(m.number)
    onOpen(m.number).then(closeSheet, () => setBusy(''))
  }
  const n = (data?.items.length ?? 0) + (data?.hidden ?? 0)
  return (
    <>
      <SheetHead icon="people" title="Anggota grup" sub={`${group}${data ? ` · ${n} anggota` : ''}`} onClose={closeSheet} />
      <div className="sec mem-sheet">
        <input className="mem-find" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Cari nama atau nomor" aria-label="Cari anggota" autoFocus />
        {isLoading && <div className="mem-none">Memuat anggota…</div>}
        {data && list.length === 0 && <div className="mem-none">{q ? 'Tidak ada anggota yang cocok' : 'Belum ada anggota yang terbaca'}</div>}
        <ul className="mem-list">
          {list.map((m) => (
            <li key={m.number}>
              <button type="button" onClick={() => open(m)} disabled={!!busy} title={m.thread_id ? 'Buka chat' : 'Mulai chat — belum ada pesan, tidak ada yang terkirim'}>
                <span className="av">{m.name.replace(/^\+/, '').slice(0, 1).toUpperCase()}</span>
                <span className="mem-who"><b>{m.name}</b><small>{fmtPhone(m.number)}</small></span>
                {m.admin && <span className="ibr-tag grp">Admin</span>}
                {m.gsi && <span className="ibr-tag">GSI</span>}
                <span className="mem-go">{busy === m.number ? 'Membuka…' : m.thread_id ? 'Chat' : 'Mulai chat'}<Icon name="chat" /></span>
              </button>
            </li>
          ))}
        </ul>
        {!!data?.hidden && <div className="mem-none">{data.hidden} anggota menyembunyikan nomornya di WhatsApp — tidak bisa dibuka dari sini</div>}
      </div>
    </>
  )
}
