import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { PasswordInput } from '../../components/PasswordInput'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { ColumnTable, type Column } from '../../components/DataTable'
import { useMe } from '../../app/queries'
import { field, fmtWA, useBranchMaster, useRoles, useUsers, type UserRow } from './api'

const WA_STATE: Record<string, [string, 'good' | 'warn' | 'bad' | 'neutral']> = {
  connected: ['WA terhubung', 'good'], pairing: ['WA menunggu ditautkan', 'warn'], disconnected: ['WA terputus', 'bad'], logged_out: ['WA keluar', 'bad'], unpaired: ['WA belum ditautkan', 'neutral'],
}

function UserSheet({ user }: { user?: UserRow }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: me } = useMe()
  const { data: cat } = useRoles()
  const { data: branches = [] } = useBranchMaster()
  const roles = (cat?.items ?? []).filter((r) => r.active && (!r.policies || me?.edit_policies))
  const [f, setF] = useState({ email: user?.email ?? '', name: user?.name ?? '', role_key: user?.role_key ?? '', branch: user?.branch && user.branch !== 'Semua cabang' ? user.branch : '', password: '' })
  const role = cat?.items.find((r) => r.key === f.role_key)
  const save = useMutation({
    mutationFn: () => {
      const body = { name: f.name, role_key: f.role_key, branch: f.branch, password: f.password || undefined }
      return user ? api.put(`/users/${user.id}`, body) : api.post('/users', { ...body, email: f.email, password: f.password })
    },
    onSuccess: () => {
      toast(user ? 'Pengguna diperbarui' : 'Pengguna ditambahkan')
      // a new name shows at once in the rail (me), Claude connections (mcp) and the sales lists
      for (const k of ['users', 'roles', 'branches', 'wa', 'me', 'mcp', 'sales', 'sales-map']) qc.invalidateQueries({ queryKey: [k] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const toggle = useMutation({
    mutationFn: () => api.put(`/users/${user!.id}`, { active: !user!.active }),
    onSuccess: () => { toast(user!.active ? 'Pengguna dinonaktifkan' : 'Pengguna diaktifkan'); qc.invalidateQueries({ queryKey: ['users'] }); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const ok = f.name.trim() && f.role_key && (user || (f.email.includes('@') && f.password.length >= 10))
  return (
    <>
      <SheetHead icon="people" title={user ? user.name : 'Tambah pengguna'} sub={user ? user.email : 'Akun login GSI Orbit'} onClose={closeSheet} />
      <div className="sec frm">
        {!user && <label style={{ gridColumn: '1 / -1' }}>Email<input style={field} type="email" autoComplete="off" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} placeholder="nama@gsicctv.com" /></label>}
        <label style={{ gridColumn: '1 / -1' }}>Nama<input style={field} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Nama lengkap" /></label>
        <label>Peran
          <select style={field} value={f.role_key} onChange={(e) => setF({ ...f, role_key: e.target.value })} aria-label="Peran">
            <option value="">Pilih peran…</option>
            {roles.map((r) => <option key={r.key} value={r.key}>{r.name}</option>)}
          </select>
        </label>
        <label>Cabang
          <select style={field} value={f.branch} onChange={(e) => setF({ ...f, branch: e.target.value })} aria-label="Cabang">
            <option value="">Semua cabang</option>
            {branches.filter((b) => b.active || b.name === f.branch).map((b) => <option key={b.id} value={b.name}>{b.name}{b.city && b.city !== b.name ? ` · ${b.city}` : ''}</option>)}
          </select>
        </label>
        {role && <small style={{ gridColumn: '1 / -1', color: 'var(--text-3)', fontWeight: 500, marginTop: -4 }}>{role.description || role.name} · {role.screens.length} halaman · {role.scope === 'own' ? 'hanya data miliknya' : 'semua data'}{role.wa_allowed ? ' · bisa memegang 1 nomor WhatsApp' : ''}</small>}
        <label style={{ gridColumn: '1 / -1' }}>{user ? 'Kata sandi baru (kosongkan bila tetap)' : 'Kata sandi'}
          <PasswordInput style={field} autoComplete="new-password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} placeholder="minimal 10 karakter" aria-label="Kata sandi" />
        </label>
        {user && (
          <div style={{ gridColumn: '1 / -1', fontSize: 12.5, color: 'var(--text-2)', background: 'var(--surface-2)', borderRadius: 10, padding: '9px 12px' }}>
            <b>WhatsApp:</b> {user.wa_linked ? `${fmtWA(user.wa_linked)} · ${WA_STATE[user.wa_state ?? '']?.[0] ?? user.wa_state}` : 'belum ada'} — nomor ditautkan di <Link to="/chat" onClick={closeSheet}>Chat → + Nomor</Link> (scan, lalu pilih pengguna)
          </div>
        )}
      </div>
      <div className="ft">
        <button className="btn primary" disabled={save.isPending || !ok} onClick={() => save.mutate()}><Icon name="check" />Simpan</button>
        {user && <button className="btn ghost" style={{ color: user.active ? 'var(--bad)' : undefined }} onClick={() => toggle.mutate()}>{user.active ? 'Nonaktifkan' : 'Aktifkan'}</button>}
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
      </div>
    </>
  )
}

const byText = (f: (u: UserRow) => string) => (a: UserRow, b: UserRow) => f(a).localeCompare(f(b), 'id')

/** Master data → Pengguna: login accounts, their role and branch; their WhatsApp number comes from Chat. A data table
 * (20 Oct): search, sort per column, pages of 10/25/50. */
export function UsersPage() {
  const { openSheet } = useFeedback()
  const { data = [] } = useUsers()
  const columns: Column<UserRow>[] = [
    { key: 'name', label: 'Nama', sort: byText((u) => u.name), cell: (u) => <><b>{u.name}</b><div className="mono">{u.email}{u.has_password ? '' : ' · belum punya kata sandi'}</div></> },
    { key: 'role', label: 'Peran', sort: byText((u) => u.role_name), cell: (u) => <Pill tone={u.role === 'ceo' ? 'accent' : u.role === 'sales' ? 'neutral' : 'indigo'}>{u.role_name}</Pill> },
    { key: 'branch', label: 'Cabang', sort: byText((u) => u.branch ?? 'Semua cabang'), cell: (u) => u.branch ?? 'Semua cabang' },
    { key: 'wa', label: 'WhatsApp', sort: byText((u) => u.wa_linked ?? '~'), cell: (u) => u.wa_linked
      ? <><span className="mono">{fmtWA(u.wa_linked)}</span><div><Pill tone={WA_STATE[u.wa_state ?? '']?.[1] ?? 'neutral'}>{WA_STATE[u.wa_state ?? '']?.[0] ?? u.wa_state}</Pill></div></>
      : <span style={{ color: 'var(--text-3)', fontSize: 12.5 }}>{u.wa_allowed ? 'belum ada' : '—'}</span> },
    { key: 'status', label: 'Status', sort: (a, b) => Number(b.active) - Number(a.active), cell: (u) => <Pill tone={u.active ? 'good' : 'neutral'}>{u.active ? 'Aktif' : 'Nonaktif'}</Pill> },
    { key: 'act', label: '', right: true, cell: (u) => <button className="btn quiet" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<UserSheet user={u} />)}>Ubah</button> },
  ]
  return (
    <div className="card">
      <div className="card-h"><h2>Pengguna</h2><span className="meta">{data.filter((u) => u.active).length} akun aktif · {data.filter((u) => u.wa_linked).length} memegang nomor WhatsApp</span>
        <button className="btn primary" style={{ height: 32, fontSize: 12.5, marginLeft: 8 }} onClick={() => openSheet(<UserSheet />)}><Icon name="people" />Tambah pengguna</button></div>
      <ColumnTable rows={data} columns={columns} rowKey={(u) => u.id} noun="pengguna" placeholder="Cari nama, email, peran, cabang, nomor WhatsApp…"
        text={(u) => [u.name, u.email, u.role_name, u.branch ?? 'Semua cabang', u.wa_linked ?? '', u.active ? 'aktif' : 'nonaktif'].join(' ')}
        initial={{ column: 'name', dir: 'asc' }} empty="Belum ada pengguna." rowClass={(u) => (u.active ? undefined : 'is-off')} />
    </div>
  )
}
