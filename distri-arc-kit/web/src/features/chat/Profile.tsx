import { useState, type MouseEvent, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { fmtPhone } from './inbox'
import './profile.css'

// Foto & profil WhatsApp (ADR 0071): a chat's avatar is the contact's or group's WhatsApp photo (the initials when
// there is none), and a click on it opens their profile like WhatsApp does, with the groups they share with GSI.

/** Threads whose photo did not load: the initials, without asking again while the page is open. */
const noPhoto = new Set<string>()

/** The avatar of a chat: its WhatsApp photo, else `fallback` (initials or the group icon). A click opens the profile. */
export function ChatAvatar({ id, group, fallback, className = '', onOpen }: {
  id: string; group: boolean; fallback: ReactNode; className?: string; onOpen?: () => void
}) {
  const [failed, setFailed] = useState(() => noPhoto.has(id))
  const img = !failed && (
    <img src={`/api/chat/threads/${id}/photo`} alt="" loading="lazy" decoding="async" onError={() => { noPhoto.add(id); setFailed(true) }} />
  )
  const cls = `av ${group ? 'grp' : ''} ${img ? 'has-photo' : ''} ${className}`.trim()
  if (!onOpen) return <span className={cls}>{img || fallback}</span>
  const open = (e: MouseEvent | React.KeyboardEvent) => { e.stopPropagation(); e.preventDefault(); onOpen() }
  return (
    <span className={`${cls} av-click`} role="button" tabIndex={0} title="Lihat profil" aria-label="Lihat profil WhatsApp"
      onClick={open} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') open(e) }}>
      {img || fallback}
    </span>
  )
}

interface ProfileData {
  kind: string
  title: string
  phone_name?: string
  account_label?: string
  number?: string
  name?: string
  about?: string
  business?: boolean
  members?: number
  groups?: { jid: string; name: string; thread_id?: string; kind?: string; members: number; admin?: boolean; via: string }[]
}

/** The WhatsApp profile of a chat's contact or group: photo, name, number, Info, and the groups in common. */
export function ProfileSheet({ id, title, group, fallback }: { id: string; title: string; group: boolean; fallback: ReactNode }) {
  const { closeSheet } = useFeedback()
  const navigate = useNavigate()
  const [big, setBig] = useState(true)
  const { data: p, isLoading, error } = useQuery({
    queryKey: ['chat', 'profile', id],
    queryFn: () => api.get<ProfileData>(`/chat/threads/${id}/profile`),
    staleTime: 5 * 60_000,
  })
  const waName = p?.name && p.name !== title ? p.name : ''
  const groups = p?.groups ?? []
  return (
    <>
      <SheetHead icon={group ? 'people' : 'phone'} title={group ? 'Info grup' : 'Info kontak'} sub="Dari profil WhatsApp" onClose={closeSheet} />
      <div className="sec wa-prof">
        <div className="wa-prof-photo">
          {big ? <img src={`/api/chat/threads/${id}/photo?full=1`} alt={`Foto profil ${title}`} onError={() => setBig(false)} /> : <span className={`wa-prof-ph ${group ? 'grp' : ''}`}>{fallback}</span>}
        </div>
        <h3 className="wa-prof-name">{title}</h3>
        {!group && p?.number && <div className="wa-prof-num">{fmtPhone(p.number)}</div>}
        {group && p?.members != null && <div className="wa-prof-num">Grup · {p.members} anggota</div>}
        {waName && <div className="wa-prof-wa">~{waName}{p?.business && <span className="pill accent">Bisnis</span>}</div>}
        {!waName && p?.business && <div className="wa-prof-wa"><span className="pill accent">Akun bisnis</span></div>}
        {isLoading && <p className="wa-prof-note">Memuat profil…</p>}
        {error && <p className="wa-prof-note">{(error as Error).message}</p>}
        {!group && p && (
          <section className="wa-prof-sec">
            <h4>Info</h4>
            <p>{p.about || <span className="wa-prof-muted">Tidak ada info, atau disembunyikan oleh pengaturan privasinya</span>}</p>
          </section>
        )}
        {!group && p && (
          <section className="wa-prof-sec">
            <h4>{groups.length} grup bersama</h4>
            {groups.length === 0 && <p className="wa-prof-muted">Tidak ada grup bersama dengan nomor GSI yang Anda pegang (atau nomornya belum terhubung).</p>}
            <ul className="wa-prof-groups">
              {groups.map((g) => (
                <li key={g.jid}>
                  <button type="button" disabled={!g.thread_id} onClick={() => { if (g.thread_id) { closeSheet(); navigate(`/chat/${g.thread_id}`, { state: { follow: true } }) } }}
                    title={g.thread_id ? 'Buka chat grup ini' : 'Chat grup ini belum tersimpan di GSI Orbit'}>
                    <span className="av grp"><Icon name="people" /></span>
                    <span className="wa-prof-g">
                      <b>{g.name || 'Grup tanpa nama'}{g.admin && <em>admin</em>}</b>
                      <small>{[`${g.members} anggota`, g.kind === 'external' ? 'grup pelanggan' : g.kind === 'internal' ? 'grup internal' : '', `lewat ${g.via}`].filter(Boolean).join(' · ')}</small>
                    </span>
                    {g.thread_id && <Icon name="chev" />}
                  </button>
                </li>
              ))}
            </ul>
          </section>
        )}
      </div>
    </>
  )
}
