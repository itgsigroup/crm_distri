import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { WANumber } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useMe, useSales, useWAStatus } from '../../app/queries'

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
          : method === 'qr' ? (qr ? <img src={qr} alt="QR WhatsApp" className="pair-qr" /> : <div className="pair-wait">Menyiapkan QR…</div>)
            : (code ? <div className="pair-code" aria-label="Kode tautan">{code}</div> : <div className="pair-wait">Meminta kode ke WhatsApp…</div>)}
        {method === 'qr' ? (
          <ol><li>Buka WhatsApp di HP nomor {n?.masked}</li><li>Setelan → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b></li><li>Arahkan kamera ke kode ini</li></ol>
        ) : (
          <ol><li>Buka WhatsApp di HP nomor {n?.masked}</li><li>Setelan → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b></li><li>Pilih <b>Tautkan dengan nomor telepon saja</b> (di bawah kamera)</li><li>Ketik kode di atas</li></ol>
        )}
        <span className="exp">{method === 'qr' ? 'QR diperbarui otomatis' : 'kode berlaku beberapa menit — minta ulang bila kedaluwarsa'} · HP tetap menerima notifikasi seperti biasa</span>
      </div>
    </>
  )
}

/** Numbers and how to link them: the Chat page's empty state, and the "+ Nomor" sheet. */
export function ConnectPanel() {
  const { data: me } = useMe()
  const admin = me?.role === 'ceo' || me?.role === 'admin'
  const { data: status } = useWAStatus()
  const { data: sales = [] } = useSales()
  const { openSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const [f, setF] = useState({ wa_number: '', label: '', sales_name: '' })
  const [busy, setBusy] = useState(false)
  const items = status?.items ?? []
  const connected = items.filter((n) => n.state === 'connected')
  const add = () => {
    setBusy(true)
    api.post<{ wa_number: string }>('/wa/numbers', f).then((r) => {
      setF({ wa_number: '', label: '', sales_name: '' })
      qc.invalidateQueries({ queryKey: ['wa'] })
      openSheet(<PairSheet wa={r.wa_number} />)
    }, (e: Error) => toast(e.message)).finally(() => setBusy(false))
  }
  return (
    <div className="connect">
      <div className="connect-h">
        <span className="connect-ic"><Icon name="chat" /></span>
        <div>
          <h2>{connected.length ? 'WhatsApp terhubung — menunggu pesan' : 'Hubungkan WhatsApp'}</h2>
          <p>{connected.length
            ? 'Riwayat chat 30 hari terakhir sedang disinkronkan dari HP (beberapa menit). Pesan baru langsung tampil di sini.'
            : 'Chat di sini adalah WhatsApp asli nomor tim: ditautkan sebagai perangkat (seperti WhatsApp Web) lewat Baileys. Pesan masuk tampil otomatis, balasan dikirim dari nomor itu lewat penjaga anti-blokir.'}</p>
        </div>
      </div>
      {items.length > 0 && (
        <ul className="nums">
          {items.map((n) => (
            <li key={n.wa_number}>
              <span className="av">{labelOf(n).slice(0, 2).toUpperCase()}</span>
              <div><b>{labelOf(n)}{n.sales && !labelOf(n).includes(n.sales) ? ` · ${n.sales}` : ''}</b><span className="no">{n.masked}{n.limits ? ` · hari ini ${n.limits.today}/${n.limits.per_day}${n.limits.warmup ? ' · pemanasan' : ''}` : ''}</span></div>
              <div className="st">
                <Pill tone={STATE[n.state]?.[1] ?? 'neutral'}>{STATE[n.state]?.[0] ?? n.state}</Pill>
                {n.state !== 'connected' && <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<PairSheet wa={n.wa_number} />)}><Icon name="qr" />Tautkan</button>}
              </div>
            </li>
          ))}
        </ul>
      )}
      {admin ? (
        <form className="wa-add connect-add" onSubmit={(e) => { e.preventDefault(); if (f.wa_number.trim() && f.label.trim()) add() }}>
          <input value={f.wa_number} onChange={(e) => setF({ ...f, wa_number: e.target.value })} placeholder="Nomor WhatsApp, mis. 0812 3456 7890" aria-label="Nomor WhatsApp" inputMode="tel" />
          <input value={f.label} onChange={(e) => setF({ ...f, label: e.target.value })} placeholder="Label, mis. Andi Sales / CS Kantor" aria-label="Label nomor" />
          <select value={f.sales_name} onChange={(e) => setF({ ...f, sales_name: e.target.value })} aria-label="Pemilik nomor">
            <option value="">Nomor tim (tanpa sales)</option>
            {sales.map((x) => <option key={x.key} value={x.name}>{x.name} · {x.branch}</option>)}
          </select>
          <button className="btn primary" type="submit" disabled={busy || !f.wa_number.trim() || !f.label.trim()}><Icon name="plug" />Tambah &amp; tautkan</button>
        </form>
      ) : items.length === 0 && <p className="connect-note">Belum ada nomor WhatsApp atas nama Anda — minta admin menambahkan nomor Anda di halaman ini.</p>}
      <ul className="connect-rules">
        <li><Icon name="shield" /><span><b>Hanya membalas</b> kontak yang pernah menghubungi nomor itu — tidak ada pesan pertama ke kontak dingin, tidak ada broadcast</span></li>
        <li><Icon name="refresh" /><span><b>Seperti manusia:</b> membaca chat dulu, "mengetik…", jeda acak, maksimal 20 pesan/jam & 120/hari per nomor, jam tenang 21.00–07.00</span></li>
        <li><Icon name="lock" /><span><b>Setiap kirim diputuskan manusia</b> — AI hanya mengusulkan; nomor baru tertaut melewati masa pemanasan 7 hari</span></li>
        <li><Icon name="phone" /><span>Pakai <b>nomor kerja yang sudah lama aktif</b>; HP tetap dipakai seperti biasa dan harus online minimal sekali tiap 14 hari</span></li>
      </ul>
    </div>
  )
}

export function ConnectSheet() {
  const { closeSheet } = useFeedback()
  return (
    <>
      <SheetHead icon="chat" title="Nomor WhatsApp" sub="Tambah dan tautkan nomor · banyak nomor sekaligus" onClose={closeSheet} />
      <div className="sec"><ConnectPanel /></div>
    </>
  )
}
