import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { api } from '../../api/client'
import type { WANumber } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useMe, useWAStatus } from '../../app/queries'
import { useUsers } from '../master/api'

export const labelOf = (n: WANumber) => (n.label || n.sales || n.masked).replace(/^Nomor\s+/, '')
const STATE: Record<string, [string, 'good' | 'warn' | 'bad' | 'neutral']> = {
  connected: ['Terhubung', 'good'], pairing: ['Menunggu ditautkan', 'warn'], disconnected: ['Terputus', 'bad'], logged_out: ['Keluar dari perangkat', 'bad'], unpaired: ['Belum ditautkan', 'neutral'],
}

type Method = 'qr' | 'code'

/** Links one number as a WhatsApp companion device (Baileys): scan a QR, or type a code on the phone — for a
 * sales who only has the phone in hand. Closes itself once the phone is linked. */
export function PairSheet({ wa, method: initial = 'qr' }: { wa: string; method?: Method }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data } = useWAStatus()
  const [method, setMethod] = useState<Method>(initial)
  const [err, setErr] = useState('')
  const n = data?.items.find((x) => x.wa_number === wa)
  const codeOK = data?.transport === 'baileys'
  const request = (m: Method) => api.post('/wa/pair', { wa_number: wa, method: m === 'code' ? 'code' : '' }).then(() => qc.invalidateQueries({ queryKey: ['wa'] }), (e: Error) => setErr(e.message))
  const start = (m: Method) => {
    setMethod(m)
    setErr('')
    request(m)
  }
  useEffect(() => { request(initial) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (n?.state === 'connected') {
      toast(`${labelOf(n)} terhubung · riwayat chat 30 hari sedang disinkronkan`)
      closeSheet()
    }
  }, [n?.state]) // eslint-disable-line react-hooks/exhaustive-deps
  const code = method === 'code' ? n?.pair_code : undefined
  const qr = method === 'qr' ? n?.qr_png : undefined
  const linking = !!n?.linking
  return (
    <>
      <SheetHead icon="qr" title={`Tautkan ${n ? labelOf(n) : ''}`} sub={n ? `${n.masked} · perangkat tertaut seperti WhatsApp Web · penjaga anti-blokir aktif` : ''} onClose={closeSheet} />
      <div className="sec">
        <div className="seg" style={{ marginBottom: 4 }}>
          <button className={method === 'qr' ? 'is-active' : ''} onClick={() => start('qr')}><Icon name="qr" />Pindai QR</button>
          <button className={method === 'code' ? 'is-active' : ''} disabled={!codeOK} onClick={() => start('code')} title={codeOK ? '' : 'Hanya untuk transport Baileys'}><Icon name="phone" />Pakai kode di HP</button>
        </div>
      </div>
      <div className="sec qr">
        {err ? <div className="pair-wait" style={{ color: 'var(--bad)' }}>{err}</div>
          : linking ? <PairLoading title={method === 'qr' ? 'QR berhasil dipindai' : 'Kode diterima'} />
            : method === 'qr' ? (qr ? <img src={qr} alt="QR WhatsApp" className="pair-qr" /> : <PairLoading title="Menyiapkan QR…" small />)
              : (code ? <div className="pair-code" aria-label="Kode tautan">{code}</div> : <PairLoading title="Meminta kode ke WhatsApp…" small />)}
        {linking ? null : method === 'qr' ? (
          <ol><li>Buka WhatsApp di HP nomor {n?.masked}</li><li>Setelan → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b></li><li>Arahkan kamera ke kode ini</li></ol>
        ) : (
          <ol><li>Buka WhatsApp di HP nomor {n?.masked}</li><li>Setelan → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b></li><li>Pilih <b>Tautkan dengan nomor telepon saja</b> (di bawah kamera)</li><li>Ketik kode di atas</li></ol>
        )}
        <span className="exp">{method === 'qr' ? 'QR diperbarui otomatis' : 'kode berlaku beberapa menit — minta ulang bila kedaluwarsa'} · HP tetap menerima notifikasi seperti biasa</span>
      </div>
    </>
  )
}

/** Spinner while the QR is prepared, and after it is scanned while the phone logs in and the first sync starts. */
function PairLoading({ title, small = false }: { title: string; small?: boolean }) {
  return (
    <div className={`pair-loading${small ? ' small' : ''}`} role="status" aria-live="polite">
      <span className="pair-spin" aria-hidden="true" />
      <b>{title}</b>
      {!small && <span>Menautkan perangkat dan mengambil data chat… biarkan WhatsApp di HP tetap terbuka.</span>}
    </div>
  )
}

interface LinkState { session_id: string; method: 'qr' | 'code'; state: 'pairing' | 'connected' | 'failed'; qr_png?: string; pair_code?: string; linking?: boolean; wa_number?: string; masked?: string; user_id?: string | null; error?: string | null }

/** Picks the user who holds a linked number (one user, one number), or releases it. */
function AssignForm({ wa, masked, current, onDone }: { wa: string; masked: string; current?: string | null; onDone: () => void }) {
  const { data: me } = useMe()
  const manager = !!me?.screens.includes('users')
  const { data: users = [] } = useUsers(manager)
  const { data: status } = useWAStatus()
  const { toast } = useFeedback()
  const qc = useQueryClient()
  const [userId, setUserId] = useState(current ?? '')
  const [busy, setBusy] = useState(false)
  const free = users.filter((u) => u.active && (u.wa_allowed || u.role === 'ceo') && (!u.wa_linked || u.wa_linked === wa))
  const assign = (id: string | null) => {
    setBusy(true)
    api.put<{ message?: string }>(`/wa/numbers/${wa}/user`, { user_id: id }).then((r) => {
      toast(r.message ?? 'Disimpan')
      for (const k of ['wa', 'users', 'chat']) qc.invalidateQueries({ queryKey: [k] })
      onDone()
    }, (e: Error) => toast(e.message)).finally(() => setBusy(false))
  }
  if (!manager) {
    const mine = status?.items.find((n) => n.wa_number === wa)?.mine
    return mine ? <p className="connect-note">Nomor ini sudah Anda pegang.</p>
      : <button className="btn primary" disabled={busy || !me} onClick={() => assign(me!.id)}><Icon name="check" />Jadikan nomor saya ({me?.name})</button>
  }
  return (
    <div className="assign">
      <label>Pemegang nomor {masked}
        <select value={userId} onChange={(e) => setUserId(e.target.value)} aria-label="Pengguna pemegang nomor">
          <option value="">{free.length ? 'Pilih nama pengguna…' : 'Semua pengguna sudah memegang nomor'}</option>
          {free.map((u) => <option key={u.id} value={u.id}>{u.name} · {u.role_name}{u.branch && u.branch !== 'Semua cabang' ? ` · ${u.branch}` : ''}</option>)}
        </select>
      </label>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <button className="btn primary" disabled={busy || !userId || userId === current} onClick={() => assign(userId)}><Icon name="check" />Simpan pemegang</button>
        {current && <button className="btn ghost" disabled={busy} onClick={() => assign(null)}>Lepas dari pengguna</button>}
      </div>
      <small>Pengguna belum ada? Tambahkan di <Link to="/pengguna">Master data → Pengguna</Link>; peran yang boleh memegang WhatsApp diatur di Peran &amp; akses.</small>
    </div>
  )
}

/** Chat → + Nomor: scan WhatsApp (QR, or a code typed on the phone), the phone reports its number, then pick the
 * user who holds it. */
export function LinkSheet() {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: wa } = useWAStatus()
  const [method, setMethod] = useState<'qr' | 'code'>('qr')
  const [phone, setPhone] = useState('')
  const [session, setSession] = useState('')
  const [busy, setBusy] = useState(false)
  const { data: link } = useQuery({
    queryKey: ['wa', 'link', session], enabled: !!session,
    queryFn: () => api.get<LinkState>(`/wa/links/${session}`),
    refetchInterval: (q) => (q.state.data?.state === 'pairing' || !q.state.data ? 2000 : false),
  })
  useEffect(() => {
    if (link?.state === 'connected') qc.invalidateQueries({ queryKey: ['wa', 'status'] })
  }, [link?.state, qc])
  const start = (m: 'qr' | 'code') => {
    setBusy(true)
    api.post<{ session_id: string }>('/wa/links', { method: m, phone }).then((r) => setSession(r.session_id), (e: Error) => toast(e.message)).finally(() => setBusy(false))
  }
  const codeOK = wa?.transport === 'baileys' || wa?.transport === 'fake'
  return (
    <>
      <SheetHead icon="qr" title={link?.state === 'connected' ? `Terhubung · ${link.masked}` : 'Hubungkan nomor WhatsApp'} sub={link?.state === 'connected' ? 'Langkah 2 dari 2 · pilih pengguna yang memegang nomor ini' : 'Langkah 1 dari 2 · tautkan HP sebagai perangkat (seperti WhatsApp Web) · penjaga anti-blokir aktif'} onClose={closeSheet} />
      {!session && (
        <div className="sec">
          <div className="seg" style={{ marginBottom: 10 }}>
            <button className={method === 'qr' ? 'is-active' : ''} onClick={() => setMethod('qr')}><Icon name="qr" />Pindai QR</button>
            <button className={method === 'code' ? 'is-active' : ''} disabled={!codeOK} onClick={() => setMethod('code')}><Icon name="phone" />Pakai kode di HP</button>
          </div>
          {method === 'code' && <label className="link-phone">Nomor WhatsApp yang akan ditautkan<input value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="mis. 0812 3456 7890" inputMode="tel" aria-label="Nomor untuk kode" /></label>}
          <p style={{ fontSize: 13, color: 'var(--text-2)', margin: '8px 0 12px' }}>{method === 'qr' ? 'QR muncul di sini; pindai dari HP pemilik nomor. Nomornya terbaca otomatis setelah tertaut.' : 'Kode 8 karakter muncul di sini; ketik di HP yang nomornya diisi di atas — cocok untuk sales yang hanya memegang HP.'}</p>
          <button className="btn primary" disabled={busy || (method === 'code' && phone.replace(/\D/g, '').length < 10)} onClick={() => start(method)}><Icon name="plug" />{busy ? 'Menyiapkan…' : method === 'qr' ? 'Tampilkan QR' : 'Minta kode'}</button>
        </div>
      )}
      {session && link?.state !== 'connected' && (
        <div className="sec qr">
          {link?.state === 'failed' ? <div className="pair-wait" style={{ color: 'var(--bad)' }}>{link.error ?? 'Gagal menautkan'}</div>
            : link?.linking ? <PairLoading title={method === 'qr' ? 'QR berhasil dipindai' : 'Kode diterima'} />
              : link?.qr_png ? <img src={link.qr_png} alt="QR WhatsApp" className="pair-qr" />
                : link?.pair_code ? <div className="pair-code" aria-label="Kode tautan">{link.pair_code}</div>
                  : <PairLoading title={method === 'qr' ? 'Menyiapkan QR…' : 'Meminta kode ke WhatsApp…'} small />}
          {link?.state === 'failed' ? <button className="btn primary" onClick={() => setSession('')}>Mulai lagi</button> : link?.linking ? null : method === 'qr' ? (
            <ol><li>Buka WhatsApp di HP pemilik nomor</li><li>Setelan → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b></li><li>Arahkan kamera ke kode ini</li></ol>
          ) : (
            <ol><li>Buka WhatsApp di HP nomor {phone}</li><li>Setelan → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b></li><li>Pilih <b>Tautkan dengan nomor telepon saja</b></li><li>Ketik kode di atas</li></ol>
          )}
          <span className="exp">{link?.linking ? 'hampir selesai…' : 'menunggu HP…'} · HP tetap menerima notifikasi seperti biasa</span>
        </div>
      )}
      {link?.state === 'connected' && link.wa_number && (
        <div className="sec">
          <div className="linked-ok"><Icon name="check" /><span>WhatsApp <b>{link.masked}</b> tertaut. Riwayat chat 30 hari sedang disinkronkan.</span></div>
          <AssignForm wa={link.wa_number} masked={link.masked ?? ''} current={link.user_id ?? null} onDone={closeSheet} />
        </div>
      )}
      <div className="ft"><button className="btn quiet" onClick={closeSheet}>{link?.state === 'connected' ? 'Pilih nanti' : 'Tutup'}</button><span className="spacer" /><span className="pol"><Icon name="lock" />Satu pengguna satu nomor · kirim hanya yang disetujui manusia</span></div>
    </>
  )
}

/** Changes who holds a linked number. */
export function AssignSheet({ n }: { n: WANumber }) {
  const { closeSheet } = useFeedback()
  return (
    <>
      <SheetHead icon="people" title={`Pemegang ${n.masked}`} sub={n.user_name ? `Sekarang: ${n.user_name}` : 'Belum ada pengguna'} onClose={closeSheet} />
      <div className="sec"><AssignForm wa={n.wa_number} masked={n.masked} current={n.user_id} onDone={closeSheet} /></div>
    </>
  )
}

/** Numbers and how to link them: the Chat page's empty state, and the "Nomor WhatsApp" sheet. */
export function ConnectPanel() {
  const { data: me } = useMe()
  const { data: status } = useWAStatus()
  const { openSheet } = useFeedback()
  const items = status?.items ?? []
  const connected = items.filter((n) => n.state === 'connected')
  const manager = !!me?.screens.includes('users')
  const canLink = manager || (me?.wa_allowed !== false && !items.some((n) => n.mine))
  return (
    <div className="connect">
      <div className="connect-h">
        <span className="connect-ic"><Icon name="chat" /></span>
        <div>
          <h2>{connected.length ? `${connected.length} nomor terhubung` : 'Hubungkan nomor WhatsApp tim'}</h2>
          <p>{connected.length
            ? 'Riwayat chat 30 hari terakhir disinkronkan dari HP. Pesan baru dari semua nomor langsung tampil di sini.'
            : 'Chat di sini adalah WhatsApp asli dari banyak nomor sekaligus. Scan WhatsApp dari HP, nomornya terbaca otomatis, lalu pilih pengguna pemegangnya — satu pengguna satu nomor.'}</p>
        </div>
      </div>
      {canLink && <button className="btn primary" style={{ alignSelf: 'flex-start' }} onClick={() => openSheet(<LinkSheet />)}><Icon name="qr" />Hubungkan nomor baru (scan)</button>}
      {items.length > 0 && <div className="connect-sub">Nomor WhatsApp tim · {items.length} nomor · {connected.length} terhubung</div>}
      {items.length > 0 && (
        <ul className="nums">
          {items.map((n) => (
            <li key={n.wa_number}>
              <span className="av">{(n.user_name || n.masked).replace(/^\+/, '').slice(0, 2).toUpperCase()}</span>
              <div><b>{n.user_name || 'Belum ada pengguna'}</b><span className="no">{n.masked}{n.user_email ? ` · ${n.user_email}` : ''}{n.limits ? ` · hari ini ${n.limits.today}/${n.limits.per_day}${n.limits.warmup ? ' · pemanasan' : ''}` : ''}</span></div>
              <div className="st">
                <Pill tone={STATE[n.state]?.[1] ?? 'neutral'}>{STATE[n.state]?.[0] ?? n.state}</Pill>
                {manager && <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<AssignSheet n={n} />)}><Icon name="people" />{n.user_id ? 'Ganti pemegang' : 'Pilih pengguna'}</button>}
                {n.state !== 'connected' && <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<PairSheet wa={n.wa_number} />)}><Icon name="qr" />Tautkan ulang</button>}
              </div>
            </li>
          ))}
        </ul>
      )}
      <ul className="connect-rules">
        <li><Icon name="shield" /><span><b>Hanya membalas</b> kontak yang pernah menghubungi nomor itu — tidak ada pesan pertama ke kontak dingin, tidak ada broadcast</span></li>
        <li><Icon name="refresh" /><span><b>Seperti manusia:</b> membaca chat dulu, "mengetik…", jeda acak, maksimal 20 pesan/jam & 120/hari per nomor, jam tenang 21.00–07.00</span></li>
        <li><Icon name="lock" /><span><b>Setiap kirim diputuskan manusia</b> — AI hanya mengusulkan; nomor baru tertaut melewati masa pemanasan 7 hari</span></li>
        <li><Icon name="phone" /><span>Pakai <b>nomor kerja yang sudah lama aktif</b>; HP tetap dipakai seperti biasa dan harus online minimal sekali tiap 14 hari</span></li>
      </ul>
    </div>
  )
}

const HUES = [210, 152, 28, 280, 340, 190, 45, 120]
const hueOf = (s: string) => HUES[[...s].reduce((a, c) => a + c.charCodeAt(0), 0) % HUES.length]
const initialsOf = (n: WANumber) => labelOf(n).replace(/^(Pak|Bu|Mbak|Mas)\s+/, '').split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase() || '#'
const STATE_DOT: Record<string, string> = { connected: 'good', pairing: 'warn', disconnected: 'bad', logged_out: 'bad', unpaired: 'neutral' }

/** The WhatsApp numbers of the team, like accounts in WhatsApp Business: pick one to see only its chats (or all),
 * see which are linked and their unread counts, link one that is not, add more. */
export function NumberRail({ account, setAccount, unread }: { account: string; setAccount: (a: string) => void; unread: Record<string, number> }) {
  const { data } = useWAStatus()
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  const canAdd = !!me?.screens.includes('users') || (me?.wa_allowed !== false && !(data?.items ?? []).some((n) => n.mine))
  const items = data?.items ?? []
  const total = Object.values(unread).reduce((a, b) => a + b, 0)
  const pick = (n: WANumber) => {
    if (n.state !== 'connected' && data?.transport !== 'fake') openSheet(<PairSheet wa={n.wa_number} />)
    else setAccount(account === n.wa_number ? '' : n.wa_number)
  }
  return (
    <nav className="wa-rail" aria-label="Nomor WhatsApp">
      <button className={`wr-i all ${account === '' ? 'is-active' : ''}`} onClick={() => setAccount('')} title={`Semua nomor · ${items.length} nomor`}>
        <span className="wr-av"><Icon name="chat" /></span>
        {total > 0 && <span className="wr-un">{total > 99 ? '99+' : total}</span>}
        <span className="wr-l">Semua</span>
      </button>
      {items.length > 0 && <span className="wr-sep" />}
      {items.map((n) => (
        <button key={n.wa_number} className={`wr-i ${account === n.wa_number ? 'is-active' : ''} ${n.state !== 'connected' ? 'off' : ''}`} onClick={() => pick(n)}
          title={`${labelOf(n)} · ${n.masked} · ${STATE[n.state]?.[0] ?? n.state}${n.state !== 'connected' ? ' — klik untuk menautkan' : ''}`}>
          <span className="wr-av" style={{ background: `hsl(${hueOf(n.wa_number)} 62% 46%)` }}>{initialsOf(n)}</span>
          <span className={`dot ${STATE_DOT[n.state] ?? 'neutral'}`} />
          {(unread[n.wa_number] ?? 0) > 0 && <span className="wr-un">{unread[n.wa_number] > 99 ? '99+' : unread[n.wa_number]}</span>}
          <span className="wr-l">{labelOf(n)}</span>
        </button>
      ))}
      {canAdd && (
        <button className="wr-i add" onClick={() => openSheet(<LinkSheet />)} title="Hubungkan nomor WhatsApp baru (scan)">
          <span className="wr-av">+</span><span className="wr-l">Nomor</span>
        </button>
      )}
    </nav>
  )
}
