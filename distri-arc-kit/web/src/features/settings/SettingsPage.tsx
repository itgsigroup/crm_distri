import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { fmtRp } from '../../lib/format'
import { AGENT_NAMES } from '../../lib/i18n/id'
import { useConnections, usePolicies, useWAGroups, useWAStatus } from '../../app/queries'
import { WhatsAppPanel } from './WhatsAppPanel'
import { OdooPanel } from './OdooPanel'

type Obj = Record<string, unknown>
const num = (o: Obj | undefined, k: string, d: number) => (o && typeof o[k] === 'number' ? (o[k] as number) : d)
const dec = (n: number) => String(n).replace('.', ',')

// Pengaturan (mockup screen-conn). Values are read from the policies table; editing arrives in stage 11,
// the AI connections in stage 07 and the calibration in stage 10.
export function SettingsPage() {
  const { toast, openSheet } = useFeedback()
  const { data: pol } = usePolicies()
  const { data: wa } = useWAStatus()
  const { data: groups = [] } = useWAGroups()
  const { data: conn } = useConnections()
  const synced = (conn?.odoo.models ?? []).some((m) => m.last_run_at)
  const orbit = pol?.['orbit.thresholds']?.value as Obj | undefined
  const seg = pol?.['segment.thresholds']?.value as Obj | undefined
  const credit = pol?.['credit.rules']?.value as Obj | undefined
  const follow = pol?.['followup.rules']?.value as Obj | undefined
  const margin = pol?.['margin.floor']?.value as Obj | undefined
  const ka = (orbit?.key_account ?? {}) as Obj
  const lim = (credit?.default_limit ?? {}) as Record<string, number>
  const drift = num(orbit, 'drift', 1.2)
  const later = () => toast('Editor kebijakan tersedia di Stage 11 · berlaku di run berikutnya')
  const connected = wa?.items.filter((n) => n.state === 'connected').length ?? 0
  const internalGroups = groups.filter((g) => g.kind === 'internal').length
  return (
    <div className="ai-grid">
      <div className="card">
        <div className="card-h"><h2>Kebijakan orbit</h2><span className="meta">Dibaca semua agen</span></div>
        <ul className="rules">
          <li><div><b>Ambang lewat jadwal</b><span>Dealer dianggap lewat jadwal saat jarak sejak order terakhir &gt; {dec(drift)}× siklus ordernya; churn &gt; {dec(num(orbit, 'churn', 2))}×</span></div><div className="seg"><button className={drift === 1.2 ? 'is-active' : ''} onClick={later}>1,2×</button><button className={drift === 1.5 ? 'is-active' : ''} onClick={later}>1,5×</button></div></li>
          <li><div><b>Ambang segmen</b><span>Sering = ≥ {dec(num(seg, 'freq_per_month', 1.5))} order/bulan (siklus order ≤ {Math.round(30 / num(seg, 'freq_per_month', 1.5))} hari) · Besar = ≥ {fmtRp(num(seg, 'size_idr', 20e6))} per order · dealer baru ditetapkan setelah order ke-{num(seg, 'new_dealer_wait_orders', 2)}</span></div><button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={later}>Ubah</button></li>
          <li><div><b>Syarat Key account</b><span>Share of wallet ≥ {num(ka, 'sow_min', 50)}% dan tepat waktu ≥ {num(ka, 'on_time_min', 85)}%, order di dalam siklus order</span></div><button className="sw on" aria-label="toggle" onClick={later} /></li>
          <li><div><b>Limit default per tier</b><span>A: {fmtRp(lim.A ?? 250e6)} · B: {fmtRp(lim.B ?? 150e6)} · C: cash · dealer baru {fmtRp(lim.new ?? 25e6)} tanpa approve</span></div><button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={later}>Ubah</button></li>
          <li><div><b>Rilis di atas limit</b><span>Butuh approve CEO; AI Kredit selalu mengusulkan DP 50% atau tahan</span></div><button className="sw on" aria-label="toggle" onClick={later} /></li>
          <li><div><b>SOP-SEC-001 sebelum rilis kredit</b><span>PO diverifikasi via telepon ke nomor terdaftar · alamat kirim konsisten · tidak bisa dimatikan</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
          <li><div><b>Floor margin {num(margin, 'pct', 9)}%</b><span>Di bawah floor tidak pernah ditawarkan</span></div><button className="sw on" aria-label="toggle" onClick={later} /></li>
          <li><div><b>Follow-up terjadwal</b><span>Maksimal 1 follow-up per {num(follow, 'gap_days', 14)} hari; selalu membawa rekomendasi order, bukan sekadar "ada kebutuhan?"</span></div><button className="sw on" aria-label="toggle" onClick={later} /></li>
          <li><div><b>Share of wallet</b><span>Diestimasi dari product mix &amp; kompetitor disebut; sales mengonfirmasi 1× per kuartal</span></div><button className="sw on" aria-label="toggle" onClick={later} /></li>
        </ul>
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Sumber sinyal</h2></div>
          <div className="conn-grid" style={{ gridTemplateColumns: '1fr' }}>
            <button className="cc" onClick={() => openSheet(<OdooPanel />)}><div className="ch"><span className="lg" style={{ background: '#714B67' }}>odoo</span><div><b>Odoo Sales · Inventory · Accounting</b><small>SO, stok per cabang, harga tier, invoice, pembayaran</small></div></div><div className="cs"><span className={`dot ${synced ? 'good' : 'warn'}`} />{synced ? `Terhubung · ${conn?.odoo.mode === 'fake' ? 'data contoh' : 'baca'}${conn?.odoo.write ? ' & tulis SO draft' : ' saja'}` : 'Belum sinkron'}</div></button>
            <button className="cc" onClick={() => openSheet(<WhatsAppPanel />)}><div className="ch"><span className="lg" style={{ background: '#25D366' }}>WA</span><div><b>WhatsApp</b><small>{wa?.items.length ?? 0} nomor sales + {internalGroups} grup gudang</small></div></div><div className="cs"><span className={`dot ${connected ? 'good' : 'warn'}`} />{connected ? `Terhubung · ${wa?.transport}` : 'Belum terhubung'} · {connected}/{wa?.items.length ?? 0} nomor</div></button>
            <button className="cc" onClick={() => toast('Identifikasi nomor oleh AI Prospek di Stage 09')}><div className="ch"><span className="lg" style={{ background: '#1E88E5' }}>ID</span><div><b>Identifikasi nomor</b><small>Profil WA Business · Truecaller · Getcontact (manual)</small></div></div><div className="cs"><span className="dot good" />Hanya nomor inbound</div></button>
          </div>
        </div>
        <div className="card">
          <div className="card-h"><h2>Koneksi AI</h2><span className="meta">Mesin analisis agen</span></div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 12 }}>
            <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)' }}>Analisis via</span>
            <div className="seg"><button onClick={later}>API AI</button><button onClick={later}>MCP</button><button className="is-active" onClick={later}>Keduanya</button></div>
          </div>
          <div className="conn-grid" style={{ gridTemplateColumns: '1fr' }}>
            <button className="cc"><div className="ch"><span className="lg" style={{ background: '#D97706' }}>API</span><div><b>API AI langsung</b><small>Claude API · cadangan OpenAI · batch analisis tiap jam dari server GSI</small></div></div><div className="cs"><span className="dot warn" />Tersambung di Stage 05</div></button>
            <button className="cc"><div className="ch"><span className="lg" style={{ background: '#5E5CE6' }}>MCP</span><div><b>MCP · Distri ARC sebagai server</b><small>Claude Desktop, ChatGPT, atau agent eksternal membaca dan menganalisis data lewat tool MCP</small></div></div><div className="cs"><span className="dot warn" />Tersedia di Stage 07</div></button>
          </div>
          <ul className="rules" style={{ marginTop: 10 }}>
            <li><div><b>Keputusan tetap manusia</b><span>API maupun MCP hanya mengusulkan; setujui / tolak ada di Pusat kendali</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
          </ul>
        </div>
        <div className="card">
          <div className="card-h"><h2>Kalibrasi agen</h2><Pill tone="neutral" icon="check">Belajar dari keputusan Anda</Pill></div>
          <ul className="cal">
            {AGENT_NAMES.slice(0, 5).map((a) => <li key={a}><span>{a}</span><div className="bar"><i style={{ width: '0%' }} /></div><span className="v num">—</span></li>)}
          </ul>
          <ul className="learn" style={{ marginTop: 10 }}><li><span className="ai" /><span>Kalibrasi terisi dari keputusan setujui / edit / tolak setelah agen berjalan.</span></li></ul>
        </div>
      </div>
    </div>
  )
}

