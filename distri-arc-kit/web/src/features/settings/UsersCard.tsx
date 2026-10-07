import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useMe } from '../../app/queries'

export interface UserRow {
  id: string; email: string; name: string; role: string; role_key: string; role_name: string; wa_allowed: boolean; active: boolean; has_password: boolean
  sales_name: string | null; branch: string | null; wa_number: string | null; wa_state: string | null; wa_linked: string | null
}
export interface RoleRow {
  key: string; name: string; description: string; base: string; system: boolean; active: boolean; wa_allowed: boolean; users: number
  screens: string[]; decide: string[]; all_screens: boolean; all_decide: boolean
}
interface RoleCatalog { items: RoleRow[]; bases: { key: string; label: string; desc: string; screens: string[]; kinds: string[] }[]; screens: { key: string; label: string }[]; kinds: { key: string; label: string }[] }

export const useUsers = (enabled = true) => useQuery({ queryKey: ['users'], queryFn: () => api.get<{ items: UserRow[] }>('/users').then((r) => r.items), enabled })
export const useRoles = (enabled = true) => useQuery({ queryKey: ['roles'], queryFn: () => api.get<RoleCatalog>('/roles'), enabled })

const WA_STATE: Record<string, [string, 'good' | 'warn' | 'bad' | 'neutral']> = {
  connected: ['WA terhubung', 'good'], pairing: ['WA menunggu ditautkan', 'warn'], disconnected: ['WA terputus', 'bad'], logged_out: ['WA keluar', 'bad'], unpaired: ['WA belum ditautkan', 'neutral'],
}
const fmtWA = (n: string | null) => (n ? '+' + n.replace(/^(\d{2})(\d{3})(\d{4})(\d+)$/, '$1 $2-$3-$4') : '')
const field = { width: '100%', height: 36, borderRadius: 9, border: '1px solid var(--line)', padding: '0 10px', font: 'inherit', fontSize: 14, background: 'var(--surface)', color: 'var(--text)', marginTop: 4 }

function UserSheet({ user }: { user?: UserRow }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: me } = useMe()
  const { data: cat } = useRoles()
  const roles = (cat?.items ?? []).filter((r) => r.active && (r.base !== 'ceo' || me?.role === 'ceo'))
  const [f, setF] = useState({ email: user?.email ?? '', name: user?.name ?? '', role_key: user?.role_key ?? 'sales', password: '', branch: user?.branch ?? '', wa_number: user?.wa_number ?? '' })
  const role = cat?.items.find((r) => r.key === f.role_key)
  const linked = !!user?.wa_linked
  const save = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = { name: f.name, role_key: f.role_key, branch: f.branch, password: f.password || undefined }
      if (!linked) body.wa_number = f.wa_number
      return user ? api.put(`/users/${user.id}`, body) : api.post('/users', { ...body, email: f.email, password: f.password })
    },
    onSuccess: () => {
      toast(user ? 'Pengguna diperbarui' : 'Pengguna ditambahkan')
      qc.invalidateQueries({ queryKey: ['users'] })
      qc.invalidateQueries({ queryKey: ['roles'] })
      qc.invalidateQueries({ queryKey: ['wa'] })
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
  return (
    <>
      <SheetHead icon="people" title={user ? user.name : 'Tambah pengguna'} sub={user ? user.email : 'Akun login Distri ARC · satu pengguna memegang satu nomor WhatsApp'} onClose={closeSheet} />
      <div className="sec frm">
        {!user && <label className="full" style={{ gridColumn: '1 / -1' }}>Email<input style={field} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} /></label>}
        <label>Nama<input style={field} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></label>
        <label>Cabang<input style={field} value={f.branch} onChange={(e) => setF({ ...f, branch: e.target.value })} placeholder="mis. Semarang" /></label>
        <label style={{ gridColumn: '1 / -1' }}>Peran
          <select style={field} value={f.role_key} onChange={(e) => setF({ ...f, role_key: e.target.value })} aria-label="Peran">
            {roles.map((r) => <option key={r.key} value={r.key}>{r.name}{r.system ? '' : ` · dasar ${cat?.bases.find((b) => b.key === r.base)?.label}`}</option>)}
          </select>
          <small style={{ color: 'var(--text-3)', fontWeight: 500 }}>{role ? `${role.description || cat?.bases.find((b) => b.key === role.base)?.desc} · ${role.screens.length} menu · ${role.decide.length} jenis keputusan` : ''}</small>
        </label>
        <label style={{ gridColumn: '1 / -1' }}>Nomor WhatsApp (satu nomor)
          <input style={field} value={f.wa_number} onChange={(e) => setF({ ...f, wa_number: e.target.value })} placeholder={role && !role.wa_allowed ? 'Peran ini tidak memegang nomor WhatsApp' : 'mis. 0812 3456 7890'} inputMode="tel" disabled={linked || (!!role && !role.wa_allowed && role.base !== 'ceo')} aria-label="Nomor WhatsApp pengguna" />
          <small style={{ color: 'var(--text-3)', fontWeight: 500 }}>{linked ? 'Nomor sedang tertaut di Chat — lepas dulu di Chat untuk menggantinya' : 'Dipakai saat menambah nomor di Chat; satu nomor hanya untuk satu pengguna'}</small>
        </label>
        <label style={{ gridColumn: '1 / -1' }}>{user ? 'Kata sandi baru (kosongkan bila tetap)' : 'Kata sandi'}<input style={field} type="password" autoComplete="new-password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} placeholder="minimal 10 karakter" /></label>
      </div>
      <div className="ft">
        <button className="btn primary" disabled={save.isPending} onClick={() => save.mutate()}><Icon name="check" />Simpan</button>
        {user && <button className="btn ghost" style={{ color: user.active ? 'var(--bad)' : undefined }} onClick={() => toggle.mutate()}>{user.active ? 'Nonaktifkan' : 'Aktifkan'}</button>}
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
      </div>
    </>
  )
}

function RoleSheet({ role, cat }: { role?: RoleRow; cat: RoleCatalog }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: me } = useMe()
  const ceo = me?.role === 'ceo'
  const [f, setF] = useState({ name: role?.name ?? '', description: role?.description ?? '', base: role?.base ?? 'sales', screens: role?.screens ?? cat.bases.find((b) => b.key === 'sales')!.screens, decide: role?.decide ?? cat.bases.find((b) => b.key === 'sales')!.kinds, wa_allowed: role?.wa_allowed ?? true })
  const base = cat.bases.find((b) => b.key === f.base)!
  const locked = !ceo || role?.base === 'ceo'
  const setBase = (k: string) => {
    const b = cat.bases.find((x) => x.key === k)!
    setF({ ...f, base: k, screens: b.screens, decide: b.kinds })
  }
  const flip = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])
  const save = useMutation({
    mutationFn: () => (role ? api.put(`/roles/${role.key}`, f) : api.post('/roles', f)),
    onSuccess: (r) => { toast((r as { message?: string }).message ?? 'Peran disimpan'); qc.invalidateQueries({ queryKey: ['roles'] }); qc.invalidateQueries({ queryKey: ['users'] }); qc.invalidateQueries({ queryKey: ['me'] }); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api.del(`/roles/${role!.key}`),
    onSuccess: () => { toast('Peran dihapus'); qc.invalidateQueries({ queryKey: ['roles'] }); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  return (
    <>
      <SheetHead icon="shield" title={role ? role.name : 'Tambah peran'} sub={role?.system ? 'Peran bawaan · jenis akses dasar tetap' : 'Peran mempersempit jenis akses dasarnya — tidak pernah melebarkan'} onClose={closeSheet} />
      <div className="sec frm">
        <label>Nama peran<input style={field} value={f.name} disabled={locked} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="mis. Sales Telemarketing" /></label>
        <label>Keterangan<input style={field} value={f.description} disabled={locked} onChange={(e) => setF({ ...f, description: e.target.value })} placeholder="mis. Follow-up dealer lewat WhatsApp" /></label>
      </div>
      <div className="sec">
        <h4>Jenis akses dasar <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· menentukan data yang boleh dilihat</span></h4>
        <div className="role-bases">
          {cat.bases.filter((b) => b.key !== 'ceo' || role?.base === 'ceo').map((b) => (
            <button key={b.key} className={`role-base ${f.base === b.key ? 'is-active' : ''}`} disabled={locked || role?.system} onClick={() => setBase(b.key)}><b>{b.label}</b><span>{b.desc}</span></button>
          ))}
        </div>
      </div>
      <div className="sec">
        <h4>Menu yang dibuka <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· juga ditegakkan di server</span></h4>
        <div className="role-checks">
          {cat.screens.filter((sc) => base.screens.includes(sc.key)).map((sc) => (
            <label key={sc.key} className="role-check"><input type="checkbox" disabled={locked} checked={f.screens.includes(sc.key)} onChange={() => setF({ ...f, screens: flip(f.screens, sc.key) })} />{sc.label}</label>
          ))}
        </div>
      </div>
      <div className="sec">
        <h4>Boleh memutuskan <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· saran AI yang boleh disetujui / ditolak</span></h4>
        <div className="role-checks">
          {cat.kinds.filter((k) => base.kinds.includes(k.key)).map((k) => (
            <label key={k.key} className="role-check"><input type="checkbox" disabled={locked} checked={f.decide.includes(k.key)} onChange={() => setF({ ...f, decide: flip(f.decide, k.key) })} />{k.label}</label>
          ))}
        </div>
      </div>
      <div className="sec">
        <ul className="rules">
          <li><div><b>Memegang nomor WhatsApp</b><span>Pengguna peran ini bisa diberi satu nomor dan membalas chat dari nomornya</span></div>
            <button className={`sw ${f.wa_allowed ? 'on' : ''}`} aria-label="Memegang nomor WhatsApp" disabled={locked} onClick={() => setF({ ...f, wa_allowed: !f.wa_allowed })} /></li>
        </ul>
      </div>
      <div className="ft">
        {!locked && <button className="btn primary" disabled={save.isPending || !f.name.trim()} onClick={() => save.mutate()}><Icon name="check" />Simpan</button>}
        {ceo && role && !role.system && <button className="btn ghost" style={{ color: 'var(--bad)' }} disabled={role.users > 0} title={role.users > 0 ? 'Masih dipakai pengguna' : ''} onClick={() => { if (window.confirm(`Hapus peran ${role.name}?`)) remove.mutate() }}>Hapus</button>}
        <button className="btn quiet" onClick={closeSheet}>{locked ? 'Tutup' : 'Batal'}</button>
        {!ceo && <span className="pol"><Icon name="lock" />Peran diubah oleh CEO</span>}
      </div>
    </>
  )
}

/** Pengaturan → Pengguna: accounts, their role and their one WhatsApp number. */
export function UsersCard() {
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  const { data } = useUsers(!!me?.manage_users)
  if (!me?.manage_users) return null
  return (
    <div className="card">
      <div className="card-h"><h2>Pengguna</h2><span className="meta">{(data ?? []).filter((u) => u.active).length} akun aktif · {(data ?? []).filter((u) => u.wa_number).length} memegang nomor WhatsApp</span><button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 8 }} onClick={() => openSheet(<UserSheet />)}><Icon name="people" />Tambah</button></div>
      <ul className="rules">
        {(data ?? []).map((u) => (
          <li key={u.id} style={u.active ? undefined : { opacity: 0.5 }}>
            <div><b>{u.name}</b><span>{u.email}{u.branch ? ` · ${u.branch}` : ''}{u.wa_number ? ` · WA ${fmtWA(u.wa_number)}` : u.wa_allowed ? ' · belum ada nomor WA' : ''}{u.has_password ? '' : ' · belum punya kata sandi'}</span></div>
            <span style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
              {u.wa_state && <Pill tone={WA_STATE[u.wa_state]?.[1] ?? 'neutral'}>{WA_STATE[u.wa_state]?.[0] ?? u.wa_state}</Pill>}
              <Pill tone={u.role === 'ceo' ? 'accent' : u.role === 'sales' ? 'neutral' : 'indigo'}>{u.role_name}</Pill>
              <button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={() => openSheet(<UserSheet user={u} />)}>Ubah</button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Pengaturan → Peran & akses: the role master (CEO edits, admin reads). */
export function RolesCard() {
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  const { data: cat } = useRoles(!!me?.manage_users)
  if (!me?.manage_users || !cat) return null
  const base = (k: string) => cat.bases.find((b) => b.key === k)?.label ?? k
  return (
    <div className="card">
      <div className="card-h"><h2>Peran &amp; akses</h2><span className="meta">{cat.items.length} peran · menu, keputusan, nomor WhatsApp</span>
        {me.role === 'ceo' && <button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 8 }} onClick={() => openSheet(<RoleSheet cat={cat} />)}><Icon name="shield" />Tambah peran</button>}</div>
      <ul className="rules">
        {cat.items.map((r) => (
          <li key={r.key} style={r.active ? undefined : { opacity: 0.5 }}>
            <div><b>{r.name}</b><span>{r.description || 'Akses dasar ' + base(r.base)} · {r.all_screens ? 'semua menu' : `${r.screens.length} menu`} · {r.all_decide ? 'semua keputusan dasar' : `${r.decide.length} jenis keputusan`}{r.wa_allowed ? ' · pegang 1 nomor WA' : ' · tanpa WA'} · {r.users} pengguna</span></div>
            <span style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              <Pill tone={r.system ? 'neutral' : 'indigo'}>{r.system ? 'Bawaan' : 'Dasar ' + base(r.base)}</Pill>
              <button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={() => openSheet(<RoleSheet role={r} cat={cat} />)}>{me.role === 'ceo' && r.base !== 'ceo' ? 'Ubah' : 'Lihat'}</button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
