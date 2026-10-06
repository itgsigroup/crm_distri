import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { fmtRp, shortDate } from '../../lib/format'
import { useCalibration, useConnections, useMe, usePolicies, useWAGroups, useWAStatus } from '../../app/queries'
import { PolicyEditor, useSavePolicy, type PolicyField } from './PolicyEditor'
import { UsersCard } from './UsersCard'
import { AutonomyCard } from './AutonomyCard'
import { WhatsAppPanel } from './WhatsAppPanel'
import { AIConnectionsCard } from './AIConnections'
import { IdentifyPanel } from './IdentifyPanel'
import { OdooPanel } from './OdooPanel'

type Obj = Record<string, unknown>
const num = (o: Obj | undefined, k: string, d: number) => (o && typeof o[k] === 'number' ? (o[k] as number) : d)
const dec = (n: number) => String(n).replace('.', ',')

// Pengaturan (mockup screen-conn): policies are edited by the CEO (schema-validated, versioned), users and roles by
// the CEO/admin, the autonomy matrix per cell by the CEO.
export function SettingsPage() {
  const { toast, openSheet } = useFeedback()
  const { data: pol } = usePolicies()
  const { data: wa } = useWAStatus()
  const { data: groups = [] } = useWAGroups()
  const { data: conn } = useConnections()
  const { data: cal } = useCalibration()
  const synced = (conn?.odoo.models ?? []).some((m) => m.last_run_at)
  const orbit = pol?.['orbit.thresholds']?.value as Obj | undefined
  const seg = pol?.['segment.thresholds']?.value as Obj | undefined
  const credit = pol?.['credit.rules']?.value as Obj | undefined
  const follow = pol?.['followup.rules']?.value as Obj | undefined
  const margin = pol?.['margin.floor']?.value as Obj | undefined
  const ka = (orbit?.key_account ?? {}) as Obj
  const lim = (credit?.default_limit ?? {}) as Record<string, number>
  const drift = num(orbit, 'drift', 1.2)
  const { data: me } = useMe()
  const save = useSavePolicy()
  const canEdit = !!me?.edit_policies
  const locked = () => toast('Hanya CEO yang mengubah kebijakan')
  const edit = (key: string, title: string, value: Obj | undefined, fields: PolicyField[]) =>
    canEdit && value ? openSheet(<PolicyEditor policyKey={key} title={title} value={value} fields={fields} />) : locked()
  const setDrift = (v: number) => (canEdit && orbit ? save.mutate({ key: 'orbit.thresholds', value: { ...orbit, drift: v } }) : locked())
  const connected = wa?.items.filter((n) => n.state === 'connected').length ?? 0
  const internalGroups = groups.filter((g) => g.kind === 'internal').length
  return (
    <div className="ai-grid">
      <div className="stack">
      <div className="card">
        <div className="card-h"><h2>Kebijakan orbit</h2><span className="meta">Dibaca semua agen</span></div>
        <ul className="rules">
          <li><div><b>Ambang lewat jadwal</b><span>Dealer dianggap lewat jadwal saat jarak sejak order terakhir &gt; {dec(drift)}× siklus ordernya; churn &gt; {dec(num(orbit, 'churn', 2))}×</span></div><div className="seg"><button className={drift === 1.2 ? 'is-active' : ''} onClick={() => setDrift(1.2)}>1,2×</button><button className={drift === 1.5 ? 'is-active' : ''} onClick={() => setDrift(1.5)}>1,5×</button></div></li>
          <li><div><b>Ambang segmen</b><span>Sering = ≥ {dec(num(seg, 'freq_per_month', 1.5))} order/bulan (siklus order ≤ {Math.round(30 / num(seg, 'freq_per_month', 1.5))} hari) · Besar = ≥ {fmtRp(num(seg, 'size_idr', 20e6))} per order · dealer baru ditetapkan setelah order ke-{num(seg, 'new_dealer_wait_orders', 2)}</span></div><button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={() => edit('segment.thresholds', 'Ambang segmen', seg, [
            { path: ['freq_per_month'], label: 'Sering', unit: 'order/bln', step: 0.1, min: 0.25, max: 8 },
            { path: ['size_idr'], label: 'Besar (per order)', unit: 'Rp', min: 1, max: 1000 },
            { path: ['new_dealer_wait_orders'], label: 'Dealer baru ditetapkan setelah order ke-', min: 1, max: 6 },
          ])}>Ubah</button></li>
          <li><div><b>Syarat Key account</b><span>Share of wallet ≥ {num(ka, 'sow_min', 50)}% dan tepat waktu ≥ {num(ka, 'on_time_min', 85)}%, order di dalam siklus order</span></div><button className="sw on" aria-label="Ubah syarat Key account" onClick={() => edit('orbit.thresholds', 'Syarat Key account', orbit, [
            { path: ['key_account', 'sow_min'], label: 'Share of wallet minimal', unit: '%', min: 10, max: 100 },
            { path: ['key_account', 'on_time_min'], label: 'Tepat waktu minimal', unit: '%', min: 50, max: 100 },
            { path: ['churn'], label: 'Ambang churn', unit: '×', step: 0.1, min: 1.2, max: 6, hint: 'Lebih dari ambang lewat jadwal' },
          ])} /></li>
          <li><div><b>Limit default per tier</b><span>A: {fmtRp(lim.A ?? 250e6)} · B: {fmtRp(lim.B ?? 150e6)} · C: cash · dealer baru {fmtRp(lim.new ?? 25e6)} tanpa approve</span></div><button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={() => edit('credit.rules', 'Limit & kredit', credit, [
            { path: ['default_limit', 'A'], label: 'Limit tier A', unit: 'Rp', min: 0 },
            { path: ['default_limit', 'B'], label: 'Limit tier B', unit: 'Rp', min: 0 },
            { path: ['default_limit', 'new'], label: 'Dealer baru tanpa approve', unit: 'Rp', min: 0 },
            { path: ['room_min'], label: 'Sisa limit tipis bila ruang <', step: 0.05, min: 0.05, max: 0.9 },
            { path: ['pay_max_days'], label: 'Pola bayar maksimal', unit: 'hari', min: 7, max: 120 },
          ])}>Ubah</button></li>
          <li><div><b>Rilis di atas limit</b><span>Butuh approve CEO; AI Kredit selalu mengusulkan DP 50% atau tahan</span></div><button className="sw on" aria-label="Rilis di atas limit" onClick={() => toast('Rilis di atas limit selalu butuh approve CEO — terkunci di kode')} /></li>
          <li><div><b>SOP-SEC-001 sebelum rilis kredit</b><span>PO diverifikasi via telepon ke nomor terdaftar · alamat kirim konsisten · tidak bisa dimatikan</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
          <li><div><b>Floor margin {num(margin, 'pct', 9)}%</b><span>Di bawah floor tidak pernah ditawarkan</span></div><button className="sw on" aria-label="Ubah floor margin" onClick={() => edit('margin.floor', 'Floor margin', margin, [{ path: ['pct'], label: 'Floor margin', unit: '%', step: 0.5, min: 3, max: 40 }])} /></li>
          <li><div><b>Follow-up terjadwal</b><span>Maksimal 1 follow-up per {num(follow, 'gap_days', 14)} hari; selalu membawa rekomendasi order, bukan sekadar "ada kebutuhan?"</span></div><button className="sw on" aria-label="Ubah follow-up terjadwal" onClick={() => edit('followup.rules', 'Follow-up terjadwal', follow, [
            { path: ['gap_days'], label: 'Jarak minimal antar follow-up', unit: 'hari', min: 3, max: 90 },
            { path: ['h_minus'], label: 'Follow-up otomatis H-', unit: 'hari', min: 0, max: 7 },
            { path: ['max_per_day_per_sales'], label: 'Maksimal kirim per nomor sales', unit: '/hari', min: 1, max: 40 },
          ])} /></li>
          <li><div><b>Share of wallet</b><span>Diestimasi dari product mix &amp; kompetitor disebut; sales mengonfirmasi 1× per kuartal</span></div><button className="sw on" aria-label="Share of wallet" onClick={() => toast('Share of wallet dihitung dari product mix & konfirmasi sales — tidak ada ambang yang diubah')} /></li>
        </ul>
      </div>
      <AutonomyCard />
      <UsersCard />
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Sumber sinyal</h2></div>
          <div className="conn-grid" style={{ gridTemplateColumns: '1fr' }}>
            <button className="cc" onClick={() => openSheet(<OdooPanel />)}><div className="ch"><span className="lg" style={{ background: '#714B67' }}>odoo</span><div><b>Odoo Sales · Inventory · Accounting</b><small>SO, stok per cabang, harga tier, invoice, pembayaran</small></div></div><div className="cs"><span className={`dot ${synced ? 'good' : 'warn'}`} />{synced ? `Terhubung · ${conn?.odoo.mode === 'fake' ? 'data contoh' : 'baca'}${conn?.odoo.write ? ' & tulis SO draft' : ' saja'}` : 'Belum sinkron'}</div></button>
            <button className="cc" onClick={() => openSheet(<WhatsAppPanel />)}><div className="ch"><span className="lg" style={{ background: '#25D366' }}>WA</span><div><b>WhatsApp</b><small>{wa?.items.length ?? 0} nomor sales + {internalGroups} grup gudang</small></div></div><div className="cs"><span className={`dot ${connected ? 'good' : 'warn'}`} />{connected ? `Terhubung · ${wa?.transport}` : 'Belum terhubung'} · {connected}/{wa?.items.length ?? 0} nomor</div></button>
            <button className="cc" onClick={() => openSheet(<IdentifyPanel />)}><div className="ch"><span className="lg" style={{ background: '#1E88E5' }}>ID</span><div><b>Identifikasi nomor</b><small>Profil WA Business · Truecaller · Getcontact (manual)</small></div></div><div className="cs"><span className="dot good" />Hanya nomor inbound</div></button>
          </div>
        </div>
        <AIConnectionsCard />
        <div className="card">
          <div className="card-h"><h2>Kalibrasi agen</h2><Pill tone="good" icon="check">Belajar dari keputusan Anda</Pill></div>
          <ul className="cal">
            {(cal?.agents ?? []).slice(0, 5).map((a) => (
              <li key={a.agent} title={`${a.accepted} disetujui · ${a.rejected} ditolak · 30 hari`}>
                <span>{a.agent}</span>
                <div className="bar"><i style={{ width: `${a.confidence ?? 0}%`, ...(a.confidence !== null && a.confidence < 60 ? { background: 'var(--warn)' } : {}) }} /></div>
                <span className="v num">{a.confidence === null ? '—' : `${a.confidence}%`}</span>
              </li>
            ))}
          </ul>
          <ul className="learn" style={{ marginTop: 10 }}>
            {(cal?.lessons ?? []).map((l) => (
              <li key={l.id}><span className="ai" /><span>{l.text}{l.suppress_until ? ` Berlaku sampai ${shortDate(l.suppress_until)}.` : ''}</span></li>
            ))}
            {(cal?.items ?? []).filter((c) => c.decision === 'rejected').slice(0, Math.max(0, 3 - (cal?.lessons ?? []).length)).map((c) => (
              <li key={c.id}><span className="ai" /><span>{c.agent}: “{c.title}” ditolak — {c.reason}.{c.suppress_until ? ` Saran serupa untuk dealer ini ditahan sampai ${shortDate(c.suppress_until)}.` : ''}</span></li>
            ))}
            {!(cal?.lessons ?? []).length && !(cal?.items ?? []).some((c) => c.decision === 'rejected') && <li><span className="ai" /><span>Kalibrasi terisi dari keputusan setujui / edit / tolak di Pusat kendali; 3 penolakan dengan alasan sama menjadi satu pelajaran.</span></li>}
          </ul>
        </div>
      </div>
    </div>
  )
}

