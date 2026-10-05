import { useState } from 'react'
import { useNavigate } from 'react-router'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { CardH, Pill, Prov } from '../../components/ui'
import { fmtRp, greeting } from '../../lib/format'
import { AGENT_NAMES, PENDING_ORCH } from '../../lib/i18n/id'
import { PipeChips } from '../../app/Dock'
import { useAgenda, useBrief, useCreditTight, useDrift, useDue, useKpi, useMe, useNow, useSegmenMovers, useStockPush } from '../../app/queries'
import { useOrchStatus } from '../../app/orch'
import { BriefPoints, DriftList, DueList, PushList, TightList, dueLabel } from './lists'
import type { AgendaRow, KPI } from '../../api/types'

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

export function ControlCenter() {
  const nav = useNavigate()
  const { toast } = useFeedback()
  const { data: me } = useMe()
  const now = useNow()
  const { data: brief } = useBrief()
  const { data: due = [] } = useDue(7)
  const { data: drift = [] } = useDrift()
  const { data: tight = [] } = useCreditTight()
  const { data: kpi } = useKpi()
  const { data: agenda = [] } = useAgenda()
  const { data: push = [] } = useStockPush()
  const { data: segMovers = [] } = useSegmenMovers()
  const orch = useOrchStatus()
  const [agentsOpen, setAgentsOpen] = useState(false)

  const c = brief?.counts
  const first = me?.name.split(' ')[0] ?? ''
  const bad = tight.filter((d) => d.metrics.credit.state === 'over limit' || d.metrics.credit.state === 'overdue')
  const signals = c ? c.wa + c.so + c.payments : 0
  const up = segMovers.filter((m) => m.kind === 'moved' && m.up).length
  const down = segMovers.filter((m) => m.kind === 'moved' && !m.up)

  return (
    <>
      <div className="cc-hero">
        <div className="cc-left">
          <div className="greet">
            <h2>{greeting(now)}, {first}.</h2>
            <p>
              {c ? <>Data masuk sejak kemarin: {c.wa} WhatsApp, {c.so} SO, {c.payments} pembayaran, dan stok {c.branches} cabang. </> : null}
              Orchestrator belum menjalankan siklus — langkah otonom dan keputusan muncul setelah siklus pertama.
            </p>
          </div>
          <div className="status-strip">
            <button onClick={() => scrollTo('queue-card')}><span className="n" style={{ background: 'var(--accent)' }}>{orch.pending}</span>Keputusan</button>
            <button onClick={() => scrollTo('due-card')}><span className="n" style={{ background: 'var(--good)' }}>{due.length}</span>Jadwal order<span className="m">· 7 hari</span></button>
            <button onClick={() => scrollTo('drift-card')}><span className="n" style={{ background: 'var(--warn)' }}>{drift.length}</span>Lewat jadwal</button>
            <button onClick={() => nav('/kredit')}><span className="n" style={{ background: 'var(--bad)' }}>{bad.length}</span>Over limit / overdue</button>
            <button onClick={() => nav('/orbit')}><span className="n" style={{ background: 'var(--indigo)' }}>◎</span>Buka Orbit</button>
          </div>
        </div>
        <div className="orch-card">
          <div className="oc-h"><span className="ai">Orchestrator</span><Pill tone="good" icon="check">Siap</Pill><span className="meta">belum ada siklus · tiap jam 06.00–20.00</span></div>
          <div className="oc-pipe"><PipeChips /></div>
          <div className="oc-foot">
            <div className="oc-nums"><span><b>{signals}</b> sinyal</span><span><b>0</b> otonom</span><span><b>{orch.pending}</b> keputusan</span><span><b>0</b> konflik diselesaikan</span></div>
            <div className="oc-btns">
              <button className="btn primary" onClick={() => toast(PENDING_ORCH)}><Icon name="refresh" />Analisis ulang</button>
              <button className="btn ghost" onClick={() => nav('/orchestrator')}>Buka Orchestrator</button>
            </div>
          </div>
        </div>
      </div>

      <div className="today">
        <div className="stack">
          <div className="card" id="plan-card">
            <div className="card-h"><h2>Rencana hari ini</h2><span className="ai" style={{ marginLeft: 6 }}>disusun Orchestrator</span><span className="meta">0 langkah</span></div>
            <ol className="plan">
              <li>
                <span className="pt">—</span>
                <div className="pb"><div className="px" style={{ color: 'var(--text-2)' }}>Belum ada rencana. Orchestrator menyusun Rencana hari ini dari siklus pertamanya: jadwal order, penagihan, dan push stok, diurutkan per jam.</div><div className="pm"><span className="ai">Orchestrator</span><span className="pill neutral">menunggu siklus</span></div></div>
                <div className="pa" />
              </li>
            </ol>
            <div className="plan-foot"><span className="pol"><Icon name="lock" />Langkah berlabel <b>otonom</b> berjalan sendiri dalam batas kebijakan; yang lain menunggu Anda.</span><button className="btn ghost" onClick={() => scrollTo('queue-card')}>Ke keputusan</button></div>
          </div>

          <div className="card" id="queue-card">
            <div className="card-h"><h2>Keputusan</h2><span className="meta">{orch.pending} item · di luar batas otonomi agen</span></div>
            <div className="queue">
              <div className="q done" style={{ opacity: 1 }}>
                <div className="qi" style={{ color: 'var(--text-3)' }}><Icon name="check" /></div>
                <div><div className="qt"><b>Belum ada keputusan</b><span className="pill neutral">{PENDING_ORCH}</span></div><div className="qd">Saran di luar batas otonomi — rilis kredit, harga di bawah tier, retur, kenaikan limit — muncul di sini lengkap dengan alasan, dampak, dan opsi.</div></div>
              </div>
            </div>
          </div>

          <div className="card brief">
            <div className="card-h"><span className="ai">Ringkasan Orchestrator · {brief ? new Date(brief.generated_at).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Jakarta' }) : '—'}</span><span className="meta">{c ? `Dari ${c.wa} WhatsApp, ${c.so} SO, ${c.payments} pembayaran, stok ${c.branches} cabang` : ''}</span></div>
            {brief && <BriefPoints brief={brief} />}
            <div className="foot">
              <Prov icon="chat">{c?.wa ?? 0} WhatsApp</Prov><Prov icon="doc">{c?.so ?? 0} SO Odoo</Prov><Prov icon="box">{c?.payments ?? 0} pembayaran</Prov><Prov icon="building">stok {c?.branches ?? 0} cabang</Prov>
              <Prov style={{ marginLeft: 'auto' }}>{brief?.source === 'template' ? 'template dari metrik' : 'confidence ' + brief?.confidence}</Prov>
            </div>
          </div>

          <div className="grid-2">
            <div className="card" id="due-card">
              <div className="card-h"><h2>Jadwal order · 7 hari</h2><span className="ai" style={{ marginLeft: 6 }}>AI Follow-up</span><span className="meta">Dealer yang <em>seharusnya</em> order minggu ini</span></div>
              <DueList items={due} />
            </div>
            <div className="card" id="drift-card">
              <div className="card-h"><h2>Lewat jadwal</h2><span className="ai" style={{ marginLeft: 6 }}>AI Follow-up</span><span className="meta">Lewat 1,2× siklus order — sebelum ordernya hilang</span></div>
              <DriftList items={drift} />
            </div>
          </div>
        </div>

        <div className="stack">
          <div className="card">
            <CardH title="KPI utama" meta="Harus bergerak seirama" />
            {kpi && <Wheels kpi={kpi} />}
            {kpi && (
              <p className="note" style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 12, lineHeight: 1.5 }}>
                Order tepat jadwal {kpi.on_schedule_pct >= kpi.targets.on_schedule_pct ? 'terjaga' : 'melambat'} ({kpi.drift_count} dealer lewat jadwal) → DSO ikut ({fmtRp(kpi.overdue_amount)} lewat tempo) → limit {kpi.tight_count} dealer tipis → order tertahan.
                {push[0] && <> Yang memutar lagi: push stok {push[0].name.toLowerCase()} ke dealer yang jadwal order.</>}
              </p>
            )}
          </div>
          <div className="card">
            <CardH title="Agenda sales" meta="dibagi Orchestrator · hari ini" />
            <Agenda rows={agenda} />
          </div>
          <div className="card">
            <CardH title="Push stok" ai="AI Stok" meta="Stok → dealer yang product mix-nya cocok" />
            <PushList items={push} />
          </div>
          <div className="card">
            <CardH title="Limit tipis" ai="AI Kredit" meta="Kredit yang menahan order" />
            <TightList items={tight} />
          </div>
          <div className={`agents-line ${agentsOpen ? 'open' : ''}`} onClick={() => setAgentsOpen(!agentsOpen)}>
            <span className="dot-live" />
            <span><b>6 agen AI</b> siap memproses {c?.wa ?? 0} WA, {c?.so ?? 0} SO, {c?.payments ?? 0} pembayaran · 0 kirim tanpa approve</span>
            <Icon name="chev" className="i chev" />
          </div>
          <div className="agents-body">
            <ul className="feed">
              <li><span className="ag">{AGENT_NAMES[0]}</span><div><b>{brief?.points[0]?.orders ?? 0} SO</b> 7 hari terakhir dari {brief?.points[0]?.order_dealers ?? 0} dealer; permintaan WA diproses setelah WhatsApp tersambung</div></li>
              <li><span className="ag">{AGENT_NAMES[1]}</span><div>{due.length} dealer jadwal order minggu ini, {drift.length} lewat jadwal{up ? `; ${up} dealer naik segmen` : ''}{down.length ? `, ${down.length} turun (${down.map((m) => m.name.replace(/^(PT|CV|UD|Toko)\s/, '') + `: ${m.from} → ${m.to}`).join(', ')})` : ''}</div></li>
              <li><span className="ag">{AGENT_NAMES[2]}</span><div>{bad.length} dealer over limit / overdue, {tight.length - bad.length} tipis</div></li>
              <li><span className="ag">{AGENT_NAMES[3]}</span><div>{push.map((p) => `${p.name} → ${(p.candidates ?? []).length} dealer`).join('; ') || 'tidak ada stok menua'}</div></li>
              <li><span className="ag">{AGENT_NAMES[4]}</span><div>{brief?.points[2]?.dealers.filter((d) => d.late_days).length ?? 0} dealer dengan invoice lewat tempo; nada pengingat mengikuti pola bayar</div></li>
              <li><span className="ag">{AGENT_NAMES[5]}</span><div>Nomor baru dikenali setelah WhatsApp tersambung (Stage 03)</div></li>
            </ul>
          </div>
        </div>
      </div>
    </>
  )
}

/** KPI wheels (mockup renderWheels). */
function Wheels({ kpi }: { kpi: KPI }) {
  const t = kpi.targets
  const W: [string, string, string, number, string, string][] = [
    ['Order tepat jadwal', kpi.on_schedule_pct + '%', '% dealer order di dalam 1,2× siklus order', kpi.on_schedule_pct, `target ≥ ${t.on_schedule_pct}%`, kpi.on_schedule_pct >= t.on_schedule_pct ? 'good' : 'warn'],
    ['DSO (order → bayar)', kpi.dso_days + ' hr', 'order → bayar, rata-rata dealer', Math.round((1 - (kpi.dso_days - 25) / 30) * 100), `target ${t.dso_days} hr`, kpi.dso_days <= t.dso_days ? 'good' : 'warn'],
    ['Perputaran stok', kpi.stock_turn_days + ' hr', 'perputaran stok', Math.round((1 - (kpi.stock_turn_days - 30) / 40) * 100), `target ${t.stock_turn_days} hr`, kpi.stock_turn_days <= t.stock_turn_days ? 'good' : 'warn'],
  ]
  const c = 2 * Math.PI * 26
  return (
    <div className="wheels">
      {W.map(([title, v, s, p, tg, k]) => (
        <div className="wheel" key={title}>
          <svg viewBox="0 0 64 64">
            <circle cx="32" cy="32" r="26" fill="none" stroke="var(--surface-3)" strokeWidth="7" />
            <circle cx="32" cy="32" r="26" fill="none" stroke={`var(--${k})`} strokeWidth="7" strokeLinecap="round" strokeDasharray={c} strokeDashoffset={c * (1 - Math.max(0.05, Math.min(1, p / 100)))} transform="rotate(-90 32 32)" />
            <text x="32" y="36" textAnchor="middle">{v}</text>
          </svg>
          <div><b>{title}</b><span>{s}</span><small>{tg}</small></div>
        </div>
      ))}
    </div>
  )
}

/** Agenda sales (mockup renderAgenda). */
function Agenda({ rows }: { rows: AgendaRow[] }) {
  const nav = useNavigate()
  return (
    <div className="agenda">
      {rows.map((r) => (
        <div className="ag-row" key={r.sales.key}>
          <div className="ag-who">
            <span className="avatar">{r.sales.initials}</span>
            <div><b>{r.sales.name}</b><span>{r.sales.branch} · {r.dealers} dealer</span></div>
            <span className="ag-n">{r.count}</span>
          </div>
          <ul>
            {(r.items ?? []).length === 0 && <li><span className="dot" style={{ background: 'var(--text-3)' }} />Tidak ada yang mendesak</li>}
            {(r.items ?? []).map((it) => (
              <li key={it.kind + it.dealer_id}>
                <span className="dot" style={{ background: it.kind === 'due' ? 'var(--good)' : it.kind === 'drift' ? 'var(--warn)' : 'var(--bad)' }} />
                <button className="ev" onClick={() => nav('/dealer/' + it.dealer_id)}>{it.short_name}</button>
                {it.kind === 'due' ? ` · jadwal ${dueLabel(it.due_in ?? 0)}` : it.kind === 'drift' ? ` · lewat jadwal ${it.late_days} hr` : ' · tagih dulu'}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  )
}

