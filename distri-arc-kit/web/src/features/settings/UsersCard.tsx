import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useMe } from '../../app/queries'

interface UserRow { id: string; email: string; name: string; role: string; active: boolean; has_password: boolean; sales_name: string | null; branch: string | null }

const ROLE: Record<string, string> = { ceo: 'CEO', admin: 'Admin', finance: 'Finance', sales: 'Sales', warehouse: 'Gudang' }
const ROLE_DESC: Record<string, string> = {
  ceo: 'Semua layar · kebijakan · rilis kredit',
  admin: 'Semua layar · pengguna · keputusan kecuali kredit',
  finance: 'Kredit · kas · penagihan · limit',
  sales: 'Dealer miliknya · chat nomornya',
  warehouse: 'Push stok · transfer · PO',
}

function UserSheet({ user }: { user?: UserRow }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const [f, setF] = useState({ email: user?.email ?? '', name: user?.name ?? '', role: user?.role ?? 'sales', password: '', sales: user?.sales_name ?? '' })
  const save = useMutation({
    mutationFn: () => (user ? api.put(`/users/${user.id}`, { name: f.name, role: f.role, password: f.password || undefined }) : api.post('/users', f)),
    onSuccess: () => {
      toast(user ? 'Pengguna diperbarui' : 'Pengguna ditambahkan')
      qc.invalidateQueries({ queryKey: ['users'] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const toggle = useMutation({
    mutationFn: () => api.put(`/users/${user!.id}`, { active: !user!.active }),
    onSuccess: () => {
      toast(user!.active ? 'Pengguna dinonaktifkan' : 'Pengguna diaktifkan')
      qc.invalidateQueries({ queryKey: ['users'] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const field = { width: '100%', height: 36, borderRadius: 9, border: '1px solid var(--line)', padding: '0 10px', font: 'inherit', fontSize: 14, background: 'var(--surface)', color: 'var(--text)', marginTop: 4 }
  return (
    <>
      <SheetHead icon="people" title={user ? user.name : 'Tambah pengguna'} sub={user ? user.email : 'Akun login Distri ARC'} onClose={closeSheet} />
      <div className="sec" style={{ display: 'grid', gap: 10 }}>
        {!user && <label style={{ fontSize: 12 }}>Email<input style={field} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} /></label>}
        <label style={{ fontSize: 12 }}>Nama<input style={field} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></label>
        <div style={{ fontSize: 12 }}>Peran
          <div className="chips" style={{ marginTop: 6 }}>{Object.keys(ROLE).map((r) => <button key={r} className={`chip ${f.role === r ? 'is-active' : ''}`} onClick={() => setF({ ...f, role: r })}>{ROLE[r]}</button>)}</div>
          <small style={{ color: 'var(--text-3)' }}>{ROLE_DESC[f.role]}</small>
        </div>
        {!user && f.role === 'sales' && <label style={{ fontSize: 12 }}>Nomor sales (nama di Odoo)<input style={field} value={f.sales} onChange={(e) => setF({ ...f, sales: e.target.value })} placeholder="mis. Andi" /></label>}
        <label style={{ fontSize: 12 }}>{user ? 'Kata sandi baru (kosongkan bila tetap)' : 'Kata sandi'}<input style={field} type="password" autoComplete="new-password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} placeholder="minimal 10 karakter" /></label>
      </div>
      <div className="ft">
        <button className="btn primary" disabled={save.isPending} onClick={() => save.mutate()}><Icon name="check" />Simpan</button>
        {user && <button className="btn ghost" style={{ color: user.active ? 'var(--bad)' : undefined }} onClick={() => toggle.mutate()}>{user.active ? 'Nonaktifkan' : 'Aktifkan'}</button>}
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
      </div>
    </>
  )
}

/** Pengaturan → Pengguna & peran. */
export function UsersCard() {
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  const { data } = useQuery({ queryKey: ['users'], queryFn: () => api.get<{ items: UserRow[] }>('/users').then((r) => r.items), enabled: !!me?.manage_users })
  if (!me?.manage_users) return null
  return (
    <div className="card">
      <div className="card-h"><h2>Pengguna &amp; peran</h2><span className="meta">{(data ?? []).filter((u) => u.active).length} akun aktif</span><button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 8 }} onClick={() => openSheet(<UserSheet />)}><Icon name="people" />Tambah</button></div>
      <ul className="rules">
        {(data ?? []).map((u) => (
          <li key={u.id} style={u.active ? undefined : { opacity: 0.5 }}>
            <div><b>{u.name}</b><span>{u.email} · {ROLE_DESC[u.role]}{u.has_password ? '' : ' · belum punya kata sandi'}</span></div>
            <span style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              <Pill tone={u.role === 'ceo' ? 'accent' : u.role === 'sales' ? 'neutral' : 'indigo'}>{ROLE[u.role] ?? u.role}</Pill>
              <button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={() => openSheet(<UserSheet user={u} />)}>Ubah</button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
