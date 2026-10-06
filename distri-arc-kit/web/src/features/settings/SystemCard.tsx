import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { fmtNum, fmtRp, hhmm, shortDate } from '../../lib/format'
import { useMe, useSystemStatus } from '../../app/queries'

type Tone = 'good' | 'warn' | 'bad' | 'neutral'
const when = (t?: string | null) => (t ? `${shortDate(t)} ${hhmm(t)}` : '—')

/** Pengaturan → Status sistem (CEO/admin): the details of /api/health, refreshed every 30 s. */
export function SystemCard() {
  const { data: h } = useSystemStatus()
  if (!h) return null
  const wa = h.wa ?? []
  const down = wa.filter((n) => n.state !== 'connected')
  const lastSync = (h.odoo.models ?? []).map((m) => m.last_run_at).filter(Boolean).sort().pop()
  const c = h.cycles.last
  const rows: [string, string, Tone, string][] = [
    ['Database', `PostgreSQL · ${fmtNum(h.counts.signals)} sinyal · ${fmtNum(h.counts.chat_messages)} pesan chat (partisi bulanan)`, h.db === 'ok' ? 'good' : 'bad', h.db === 'ok' ? 'Sehat' : 'Gagal'],
    ['Antrean job', `${h.queue_depth} job menunggu · alert bila > 500`, h.queue === 'ok' ? 'good' : 'warn', h.queue === 'ok' ? 'Normal' : 'Menumpuk'],
    ['WhatsApp', down.length ? `Terputus: ${down.map((n) => n.sales || n.number).join(', ')}` : `${wa.length} nomor terhubung · ${wa[0]?.transport ?? '—'}`, down.length ? 'bad' : 'good', `${wa.length - down.length}/${wa.length}`],
    ['Odoo', `${h.odoo.mode === 'fake' ? 'Data contoh' : h.odoo.mode} · sinkron terakhir ${when(lastSync)}${h.odoo.write ? ' · tulis SO draft & catatan' : ' · baca saja'}`, h.odoo.status === 'ok' ? 'good' : h.odoo.status === 'off' ? 'neutral' : 'bad', h.odoo.status === 'error' ? 'Gagal' : h.odoo.status === 'off' ? 'Mati' : 'Sehat'],
    ['AI', `${h.llm.provider} · ${h.llm.calls_today ?? 0} panggilan hari ini · ${fmtRp(h.llm.cost_today_idr ?? 0)}`, h.llm.status === 'ok' ? 'good' : 'warn', h.llm.status === 'ok' ? 'Aktif' : 'Template'],
    ['Siklus Orchestrator', c ? `#${c.number ?? '—'} · ${when(c.started_at)} · ${c.duration_ms ?? 0} ms${h.cycles.failed_streak ? ` · gagal ${h.cycles.failed_streak}× berturut` : ''}` : 'Belum ada siklus', h.cycles.failed_streak >= 2 ? 'bad' : c?.status === 'failed' ? 'warn' : 'good', c?.status === 'failed' ? 'Gagal' : 'Normal'],
    ['Outbox', Object.entries(h.outbox).map(([k, v]) => `${v} ${k}`).join(' · ') || 'Kosong', h.outbox.failed ? 'warn' : 'good', h.outbox.failed ? `${h.outbox.failed} gagal` : 'Lancar'],
  ]
  return (
    <div className="card">
      <div className="card-h"><h2>Status sistem</h2><span className="meta">v{h.version} · {hhmm(h.now)}</span></div>
      <ul className="rules">
        {rows.map(([t, d, tone, label]) => <li key={t}><div><b>{t}</b><span>{d}</span></div><Pill tone={tone}>{label}</Pill></li>)}
      </ul>
      {(h.alerts ?? []).length > 0 && (
        <ul className="learn" style={{ marginTop: 10 }}>
          {(h.alerts ?? []).map((a) => <li key={a.key}><span className="ai" /><span>{a.message} · sejak {hhmm(a.opened_at)}{a.notified_at ? ' · dikirim ke grup internal' : ''}</span></li>)}
        </ul>
      )}
      <p style={{ fontSize: 12, color: 'var(--text-3)', margin: '10px 0 0', lineHeight: 1.5 }}>Alert dikirim ke grup WhatsApp internal (bukan ke dealer): WhatsApp terputus, siklus gagal 2×, antrean &gt; 500. Metrik Prometheus di <code style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5 }}>/metrics</code>.</p>
    </div>
  )
}

function TOTPSheet({ enabled }: { enabled: boolean }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null)
  const [code, setCode] = useState('')
  const start = useMutation({ mutationFn: () => api.post<{ secret: string; uri: string }>('/auth/totp/setup', {}), onSuccess: setSetup, onError: (e: Error) => toast(e.message) })
  const confirm = useMutation({
    mutationFn: () => api.post<{ message: string }>(enabled ? '/auth/totp/disable' : '/auth/totp/enable', { code }),
    onSuccess: (r) => {
      toast(r.message)
      qc.invalidateQueries({ queryKey: ['me'] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const field = { width: 160, height: 40, borderRadius: 10, border: '1px solid var(--line)', padding: '0 12px', font: 'inherit', fontSize: 18, letterSpacing: '.2em', background: 'var(--surface)', color: 'var(--text)' }
  return (
    <>
      <SheetHead icon="lock" title={enabled ? 'Matikan 2FA' : 'Aktifkan 2FA'} sub="Kode 6 digit dari Google Authenticator, Authy, atau 1Password" onClose={closeSheet} />
      {!enabled && (
        <div className="sec">
          <h4>1 · Tambahkan ke aplikasi authenticator</h4>
          {!setup ? (
            <button className="btn primary" disabled={start.isPending} onClick={() => start.mutate()}><Icon name="lock" />Buat kunci</button>
          ) : (
            <div className="prev">
              <b>Kunci setup (ketik manual di aplikasi, jenis: berbasis waktu):</b>
              <div className="ep" style={{ marginTop: 6 }}><span style={{ fontFamily: 'var(--font-mono)', letterSpacing: '.08em', wordBreak: 'break-all' }}>{setup.secret.match(/.{1,4}/g)?.join(' ')}</span><button className="btn ghost" onClick={() => void navigator.clipboard?.writeText(setup.secret).then(() => toast('Disalin'))}>Salin</button></div>
              <p style={{ fontSize: 12, color: 'var(--text-2)', margin: '8px 0 0', wordBreak: 'break-all' }}>Atau buka tautan ini di ponsel: <code style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{setup.uri}</code></p>
            </div>
          )}
        </div>
      )}
      {(enabled || setup) && (
        <div className="sec">
          <h4>{enabled ? 'Kode saat ini' : '2 · Masukkan kode pertama'}</h4>
          <input style={field} inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} aria-label="Kode 2FA" onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} />
        </div>
      )}
      <div className="ft">
        <button className="btn primary" disabled={code.length !== 6 || confirm.isPending} onClick={() => confirm.mutate()}><Icon name="check" />{enabled ? 'Matikan 2FA' : 'Aktifkan'}</button>
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
        <span className="spacer" />
        <span className="pol"><Icon name="lock" />Kunci disimpan terenkripsi · ponsel hilang: arc ctl user totp-reset</span>
      </div>
    </>
  )
}

/** Pengaturan → Keamanan akun: 2FA TOTP for CEO and admin. */
export function SecurityCard() {
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  if (!me?.totp_available) return null
  return (
    <div className="card">
      <div className="card-h"><h2>Keamanan akun</h2><span className="meta">{me.email}</span></div>
      <ul className="rules">
        <li><div><b>Verifikasi dua langkah (2FA)</b><span>{me.totp_enabled ? 'Aktif · login meminta kode 6 digit dari aplikasi authenticator' : 'Disarankan untuk CEO & admin: kata sandi saja tidak cukup untuk akses rilis kredit dan kebijakan'}</span></div>
          <button className={`sw ${me.totp_enabled ? 'on' : ''}`} aria-label="Verifikasi dua langkah" onClick={() => openSheet(<TOTPSheet enabled={!!me.totp_enabled} />)} /></li>
        <li><div><b>Sesi login</b><span>12 jam · cookie HttpOnly · 10 percobaan gagal / 15 menit lalu dikunci</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
      </ul>
    </div>
  )
}
