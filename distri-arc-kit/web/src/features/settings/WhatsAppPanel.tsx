import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useInternalNumbers, useWAGroups, useWAStatus } from '../../app/queries'
import { AddNumberForm } from '../chat/Connect'

const STATE: Record<string, [string, 'good' | 'warn' | 'bad' | 'neutral']> = {
  connected: ['Terhubung', 'good'], pairing: ['Menunggu scan QR', 'warn'], disconnected: ['Terputus', 'bad'], logged_out: ['Keluar dari perangkat', 'bad'], unpaired: ['Belum dipasangkan', 'neutral'],
}

/** Pengaturan → Sumber sinyal → WhatsApp: numbers, QR pairing, internal numbers and groups (stage 03). */
export function WhatsAppPanel() {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: status } = useWAStatus()
  const { data: internal = [] } = useInternalNumbers()
  const { data: groups = [] } = useWAGroups()
  const unpair = useMutation({
    mutationFn: (n: string) => api.del(`/wa/numbers/${n}`),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['wa'] }); toast('Nomor dilepas · perangkat tertaut dikeluarkan') },
    onError: (e: Error) => toast(e.message),
  })
  const [no, setNo] = useState('')
  const [label, setLabel] = useState('')
  const pair = useMutation({
    mutationFn: (n: string) => api.post('/wa/pair', { wa_number: n }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['wa'] }); toast('Pairing dimulai · QR muncul di sini') },
    onError: (e: Error) => toast(e.message),
  })
  const addInternal = useMutation({
    mutationFn: () => api.post('/internal-numbers', { wa_number: no, label, department: '' }),
    onSuccess: () => { setNo(''); setLabel(''); qc.invalidateQueries({ queryKey: ['wa', 'internal'] }); toast('Nomor internal ditambahkan · DM ke nomor ini tidak dibaca') },
    onError: (e: Error) => toast(e.message),
  })
  const delInternal = useMutation({
    mutationFn: (n: string) => api.del(`/internal-numbers/${n}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['wa', 'internal'] }),
  })
  const patchGroup = useMutation({
    mutationFn: (g: { id: string; kind: string; read_enabled: boolean }) => api.patch(`/wa/groups/${g.id}`, g),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['wa', 'groups'] }); toast('Grup diperbarui') },
  })
  const pairing = status?.items.find((n) => n.qr_png)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (no.trim()) addInternal.mutate()
  }
  return (
    <>
      <SheetHead icon="chat" title="WhatsApp" sub={`Transport: ${status?.transport ?? '…'} · banyak nomor · kirim hanya lewat outbox yang disetujui`} onClose={closeSheet} />
      <div className="sec">
        <h4>Nomor WhatsApp <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· {(status?.items ?? []).filter((n) => n.state === 'connected').length}/{status?.items.length ?? 0} terhubung</span></h4>
        <ul className="nums">
          {(status?.items ?? []).map((n) => {
            const name = n.user_name || n.label || n.sales || n.masked
            const lim = n.limits
            return (
              <li key={n.wa_number}>
                <span className="av">{name.replace(/^Nomor\s+/, '').slice(0, 2).toUpperCase()}</span>
                <div>
                  <b>{name}{n.sales && !name.includes(n.sales) ? ` · ${n.sales}` : ''}</b>
                  <span className="no">{n.masked}{n.sales ? ` · ${n.branch}` : ' · nomor tim'}</span>
                  {lim && <span className="no" style={{ display: 'block' }}>hari ini {lim.today}/{lim.per_day}{lim.warmup ? ' · pemanasan nomor baru' : ''} · jam ini {lim.last_hour}/{lim.per_hour}</span>}
                </div>
                <div className="st">
                  <Pill tone={STATE[n.state][1]}>{STATE[n.state][0]}</Pill>
                  {n.state !== 'connected' && n.state !== 'pairing' && <button className="btn ghost" style={{ height: 26, fontSize: 11.5 }} onClick={() => pair.mutate(n.wa_number)}><Icon name="qr" />Pasangkan</button>}
                  {(n.state !== 'unpaired' || !n.sales_id) && <button className="btn quiet" style={{ height: 26, fontSize: 11.5 }} onClick={() => { if (window.confirm(`Lepas ${name} dari Distri ARC? Perangkat tertaut di HP ikut dikeluarkan.`)) unpair.mutate(n.wa_number) }}>Lepas</button>}
                </div>
              </li>
            )
          })}
        </ul>
        <div className="connect" style={{ marginTop: 12, gap: 10 }}><AddNumberForm /></div>
      </div>
      {pairing && (
        <div className="sec qr">
          <img src={pairing.qr_png} alt="QR WhatsApp" style={{ width: 196, height: 196, borderRadius: 14, background: '#fff', padding: 12, boxShadow: 'var(--shadow)' }} />
          <ol><li>Buka WhatsApp di ponsel {pairing.label || pairing.sales} ({pairing.masked})</li><li>Setelan → Perangkat tertaut → Tautkan perangkat</li><li>Arahkan kamera ke kode ini</li></ol>
          <span className="exp">kode diperbarui otomatis</span>
        </div>
      )}
      <div className="sec">
        <h4>Penjaga anti-blokir <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· berlaku untuk setiap nomor, setelah pesan disetujui</span></h4>
        <ul className="rules">
          <li><div><b>Hanya membalas kontak yang pernah menghubungi nomor itu</b><span>Pesan pertama ke kontak dingin adalah pemicu utama laporan spam — hubungi lewat telepon atau minta kontak menyapa dulu</span></div><Pill tone="neutral" icon="lock">Aktif</Pill></li>
          <li><div><b>Batas per nomor 20/jam, 120/hari · per chat 6/jam, jeda ≥ 20 dtk</b><span>Nomor baru tertaut: pemanasan 15 pesan/hari, naik bertahap selama 7 hari</span></div><Pill tone="neutral" icon="lock">Aktif</Pill></li>
          <li><div><b>Jam kirim 08.00–18.00 WIB · jam tenang 21.00–07.00</b><span>Di luar jam itu pesan menunggu, tidak dibuang</span></div><Pill tone="neutral" icon="lock">Aktif</Pill></li>
          <li><div><b>Teks identik ke &gt; 3 chat/jam ditolak · "STOP" / "berhenti" dihormati</b><span>Pola broadcast tidak dikirim; personalisasi pesannya</span></div><Pill tone="neutral" icon="lock">Aktif</Pill></li>
          <li><div><b>Seperti manusia: "mengetik…", jeda acak 2–6 dtk, tidak online terus</b><span>Chat dibaca (centang biru) hanya tepat sebelum membalas, tidak mengunduh media, reconnect pelan; setelah logout/diblokir tidak menyambung sendiri</span></div><Pill tone="neutral" icon="lock">Aktif</Pill></li>
        </ul>
        <p style={{ fontSize: 12, color: 'var(--text-3)', margin: '8px 0 0', lineHeight: 1.5 }}>Pakai nomor kerja yang sudah lama aktif, bukan nomor baru. WhatsApp tetap bisa membatasi nomor yang dilaporkan penerima — volume besar atau kontak baru lewat WhatsApp Cloud API resmi.</p>
      </div>
      <div className="sec">
        <h4>Nomor internal <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· DM antar nomor internal tidak pernah dibaca</span></h4>
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Nomor</th><th>Nama</th><th>Bagian</th><th /></tr></thead>
            <tbody>
              {internal.map((n) => (
                <tr key={n.wa_number}>
                  <td className="mono">{n.wa_number}</td><td>{n.label}</td><td>{n.department}{n.is_sales ? ' · sales' : ''}</td>
                  <td>{!n.is_sales && <button className="btn quiet" style={{ height: 26, fontSize: 11.5 }} onClick={() => delInternal.mutate(n.wa_number)}>Hapus</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <form className="frm" style={{ marginTop: 10 }} onSubmit={submit}>
          <label>Nomor<input value={no} onChange={(e) => setNo(e.target.value)} placeholder="0812…" /></label>
          <label>Nama<input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="mis. Gudang Yogya" /></label>
          <div className="full"><button className="btn ghost" type="submit"><Icon name="check" />Tandai internal</button></div>
        </form>
      </div>
      <div className="sec">
        <h4>Grup</h4>
        <ul className="rules">
          {groups.map((g) => (
            <li key={g.id}>
              <div><b>{g.name}</b><span>{g.kind === 'internal' ? 'Internal · hanya stok, surat jalan, jadwal, tugas' : 'Eksternal'} · {g.members ?? 0} anggota</span></div>
              <button className={`sw ${g.read_enabled ? 'on' : ''}`} aria-label="Baca grup" onClick={() => patchGroup.mutate({ id: g.id, kind: g.kind, read_enabled: !g.read_enabled })} />
            </li>
          ))}
        </ul>
      </div>
      <div className="ft"><button className="btn ghost" onClick={closeSheet}>Tutup</button><span className="spacer" /><span className="pol"><Icon name="lock" />Tidak ada broadcast · jeda acak · batas harian per nomor</span></div>
    </>
  )
}
