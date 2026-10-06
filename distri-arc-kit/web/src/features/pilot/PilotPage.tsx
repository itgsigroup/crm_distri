import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { api } from '../../api/client'
import type { PilotAgent, PilotReport } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { shortDate } from '../../lib/format'
import { useMe } from '../../app/queries'

const MODES: [PilotReport['mode'], string][] = [['off', 'Mati'], ['shadow', 'Bayangan'], ['live', 'Live']]
const pct = (v: number | null) => (v == null ? '—' : `${v}%`)
const dur = (m: number | null) => (m == null ? '—' : m >= 120 ? `${(m / 60).toFixed(1).replace('.', ',')} jam` : `${m} mnt`)

export const usePilot = () => useQuery({ queryKey: ['pilot'], queryFn: () => api.get<{ report: PilotReport; weeks: PilotReport[] }>('/pilot'), refetchInterval: 60_000 })

function usePilotAction() {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  return useMutation({
    mutationFn: ({ path, body }: { path: string; body: unknown }) => api.post<{ message: string }>(path, body),
    onSuccess: (r) => {
      toast(r.message)
      for (const k of ['pilot', 'me', 'policies']) qc.invalidateQueries({ queryKey: [k] })
    },
    onError: (e: Error) => toast(e.message),
  })
}

function Weeks({ a }: { a: PilotAgent }) {
  if (!a.weeks.length) return <span style={{ color: 'var(--text-3)' }}>—</span>
  return (
    <span style={{ display: 'inline-flex', gap: 3, alignItems: 'flex-end', height: 22 }} title={a.weeks.map((w) => `${shortDate(w.week)}: ${w.pct}% (${w.accepted}/${w.accepted + w.rejected})`).join('\n')}>
      {a.weeks.slice(-6).map((w) => <i key={w.week} style={{ width: 8, height: Math.max(3, (w.pct / 100) * 22), borderRadius: 2, background: w.pct >= 80 ? 'var(--good)' : 'var(--warn)' }} />)}
    </span>
  )
}

/** Pengaturan → Pilot (stage 14): mode, numbers per agent, KPI, audit, weekly CSV. */
export function PilotPage() {
  const { data } = usePilot()
  const { data: me } = useMe()
  const act = usePilotAction()
  const ceo = me?.role === 'ceo'
  const r = data?.report
  if (!r) return null
  const modeText = r.mode === 'shadow' ? `Mode bayangan sampai ${r.shadow_until ? shortDate(r.shadow_until) : '—'}: Orchestrator menganalisis, semua saran butuh approve, tidak ada yang dikirim` : r.mode === 'live' ? 'Live: kirim setelah disetujui · langkah otomatis hanya untuk agen yang dibuka' : 'Pilot belum berjalan · matriks otonomi berlaku penuh'
  return (
    <div className="stack">
      <div className="card">
        <div className="card-h"><h2>Pilot cabang {r.branch}</h2><span className="meta">{r.started_at ? `mulai ${shortDate(r.started_at)} · hari ke-${r.day}` : 'belum dimulai'}</span></div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
          <div className="seg">{MODES.map(([k, l]) => <button key={k} className={r.mode === k ? 'is-active' : ''} disabled={!ceo || act.isPending} onClick={() => act.mutate({ path: '/pilot/mode', body: { mode: k } })}>{l}</button>)}</div>
          <span style={{ fontSize: 12.5, color: 'var(--text-2)', flex: 1, minWidth: 220 }}>{modeText}</span>
        </div>
        <div className="kpis" style={{ marginTop: 14 }}>
          <div className="kpi"><small>Saran diterima</small><b className="num">{pct(r.accept_pct)}</b><span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>{r.decisions} keputusan manusia</span></div>
          <div className="kpi"><small>Order tepat jadwal</small><b className="num" style={{ color: r.on_schedule_pct >= r.Targets.on_schedule_pct ? 'var(--good)' : 'var(--warn)' }}>{r.on_schedule_pct}%</b><span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>target {r.Targets.on_schedule_pct}%</span></div>
          <div className="kpi"><small>DSO</small><b className="num" style={{ color: r.dso_days <= r.Targets.dso_days ? 'var(--good)' : 'var(--warn)' }}>{r.dso_days} hari</b><span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>target {r.Targets.dso_days} hari</span></div>
          <div className="kpi"><small>Lewat jadwal tertangkap</small><b className="num">{r.caught_before_churn}/{r.at_risk}</b><span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>order lagi sebelum churn · {r.churned} churn</span></div>
        </div>
      </div>

      <div className="card">
        <div className="card-h"><h2>Per agen</h2><span className="meta">{shortDate(r.from)} – {shortDate(r.to)} · buka otonomi bila ≥ 80% dua minggu berturut</span></div>
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Agen</th><th>Saran</th><th>Setuju</th><th>Edit</th><th>Tolak</th><th>Diterima</th><th>Median keputusan</th><th>Mingguan</th><th>Otonomi</th></tr></thead>
            <tbody>
              {r.agents.map((a) => (
                <tr key={a.agent}>
                  <td><b>{a.agent}</b></td>
                  <td className="num">{a.proposed}</td><td className="num">{a.approved}</td><td className="num">{a.edited}</td><td className="num">{a.rejected}</td>
                  <td className="num">{pct(a.accept_pct)}</td><td className="num">{dur(a.median_decision_min)}</td><td><Weeks a={a} /></td>
                  <td>
                    {a.unlocked ? <Pill tone="good" icon="check">Dibuka</Pill>
                      : a.eligible && r.mode === 'live' && ceo ? <button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={() => act.mutate({ path: '/pilot/unlock', body: { agent: a.agent } })}>Buka otonomi</button>
                      : <span style={{ fontSize: 11.5, color: 'var(--text-3)' }} title={a.eligible_note}>{a.eligible ? 'Memenuhi syarat' : 'Butuh approve'}</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="ai-grid">
        <div className="card">
          <div className="card-h"><h2>Audit privasi &amp; kirim</h2>{r.audit_ok ? <Pill tone="good" icon="check">Bersih</Pill> : <Pill tone="bad">Ada pelanggaran</Pill>}</div>
          <ul className="rules">
            {r.audit.map((c) => <li key={c.key}><div><b>{c.label}</b><span>{c.violations === 0 ? 'Tidak ada' : `${c.violations} kejadian — lihat RUNBOOK §8`}</span></div><Pill tone={c.violations ? 'bad' : 'good'}>{c.violations}</Pill></li>)}
          </ul>
        </div>
        <div className="card">
          <div className="card-h"><h2>Laporan mingguan</h2><a className="btn quiet" style={{ height: 26, fontSize: 12 }} href="/api/pilot/export.csv" download><Icon name="doc" />CSV s.d. hari ini</a></div>
          <ul className="rules">
            {(data?.weeks ?? []).map((w) => (
              <li key={w.from}><div><b>Minggu {shortDate(w.from)}</b><span>{pct(w.accept_pct)} diterima · {w.decisions} keputusan · tepat jadwal {w.on_schedule_pct}% · DSO {w.dso_days} hr · audit {w.audit_ok ? 'bersih' : 'GAGAL'}</span></div>
                <a className="btn ghost" style={{ height: 26, fontSize: 12 }} href={`/api/pilot/export.csv?week=${w.from.slice(0, 10)}`} download>CSV</a></li>
            ))}
            {!(data?.weeks ?? []).length && <li><div><b>Belum ada minggu tersimpan</b><span>Snapshot otomatis tiap Senin 00.45 WIB · ringkasan masuk docs/PILOT-REPORT.md</span></div></li>}
          </ul>
        </div>
      </div>
      <p style={{ fontSize: 12, color: 'var(--text-3)', margin: 0 }}><Link to="/pengaturan">← Pengaturan</Link></p>
    </div>
  )
}

/** Pengaturan card: pilot at a glance, link to the dashboard. */
export function PilotCard() {
  const { data } = usePilot()
  const r = data?.report
  if (!r) return null
  return (
    <div className="card">
      <div className="card-h"><h2>Pilot</h2><Pill tone={r.mode === 'off' ? 'neutral' : r.mode === 'shadow' ? 'warn' : 'good'}>{r.mode === 'off' ? 'Belum berjalan' : r.mode === 'shadow' ? 'Mode bayangan' : 'Live'}</Pill></div>
      <ul className="rules">
        <li><div><b>Cabang {r.branch}{r.day ? ` · hari ke-${r.day}` : ''}</b><span>{pct(r.accept_pct)} saran diterima · tepat jadwal {r.on_schedule_pct}% · DSO {r.dso_days} hr · audit {r.audit_ok ? 'bersih' : 'ada pelanggaran'}</span></div><Link className="btn quiet" style={{ height: 26, fontSize: 12 }} to="/pengaturan/pilot">Buka</Link></li>
      </ul>
    </div>
  )
}
