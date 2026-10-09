import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { Icon } from '../../components/Icon'
import { CardH, Pill, Prov } from '../../components/ui'
import { fmtRp, greeting, hhmm, shortDate } from '../../lib/format'
import { AGENT_NAMES } from '../../lib/i18n/id'
import { PipeChips } from '../../app/Dock'
import { useAgenda, useBrief, useCreditTight, useDrift, useDue, useKpi, useMe, useNow, usePlan, useQueue, useSegmenMovers, useStockPush } from '../../app/queries'
import { useOrch, useOrchStatus } from '../../app/orch'
import { PlanList, planMeta } from './Plan'
import { useMore } from '../../components/More'
import { BriefPoints, DriftList, DueList, PushList, TightList, dueLabel } from './lists'
import { Queue } from './Queue'
import type { AgendaRow, KPI } from '../../api/types'

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

export function ControlCenter() {
  const nav = useNavigate()
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
  const { reanalyze } = useOrch()
  const { data: plan } = usePlan()
  const full = orch.lastFull
  const { data: queue = [] } = useQueue()
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
              {full ? (
                <>Orchestrator sudah memproses {c?.wa ?? 0} WhatsApp, {c?.so ?? 0} SO, {c?.payments ?? 0} pembayaran, dan stok {c?.branches ?? 0} cabang. {full.auto_count ?? 0} langkah otonom, {orch.pending} menunggu keputusan Anda.</>
              ) : (
                <>
                  {c ? <>Data masuk sejak kemarin: {c.wa} WhatsApp, {c.so} SO, {c.payments} pembayaran, dan stok {c.branches} cabang. </> : null}
                  Orchestrator belum menjalankan siklus — langkah otonom dan keputusan muncul setelah siklus pertama.
                </>
              )}
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
          <div className="oc-h">
            <span className="ai">Orchestrator</span>
            {orch.running ? <Pill tone="accent" icon="refresh">Menganalisis</Pill> : <Pill tone="good" icon="check">Siap</Pill>}
            <span className="meta">{orch.run ? `siklus #${orch.run.toLocaleString('id-ID')} · ${orch.last} · ${orch.dur}` : 'belum ada siklus · tiap jam 06.00–20.00'}</span>
          </div>
          <div className="oc-pipe"><PipeChips /></div>
          <LastUpdate />
          <div className="oc-foot">
            <div className="oc-nums"><span><b>{full ? full.signals_count ?? 0 : signals}</b> sinyal</span><span><b>{full?.auto_count ?? 0}</b> otonom</span><span><b>{orch.pending}</b> keputusan</span><span><b>{full?.conflict_count ?? 0}</b> konflik diselesaikan</span></div>
            <div className="oc-btns">
              <button className="btn primary" disabled={orch.running} onClick={() => reanalyze('all')}><Icon name="refresh" />Analisis ulang</button>
              <button className="btn ghost" onClick={() => nav('/orchestrator')}>Buka Orchestrator</button>
            </div>
          </div>
        </div>
      </div>

      <div className="today">
        <div className="stack">
          <div className="card" id="plan-card">
            <div className="card-h"><h2>Rencana hari ini</h2><span className="ai" style={{ marginLeft: 6 }}>disusun Orchestrator</span><span className="meta">{planMeta(plan)}</span></div>
            <PlanList plan={plan} />
            <div className="plan-foot"><span className="pol"><Icon name="lock" />Langkah berlabel <b>otonom</b> berjalan sendiri dalam batas kebijakan; yang lain menunggu Anda.</span><button className="btn ghost" onClick={() => scrollTo('queue-card')}>Ke keputusan</button></div>
          </div>

          <div className="card" id="queue-card">
            <div className="card-h"><h2>Keputusan</h2><span className="meta">{orch.pending ? `${orch.pending} item · di luar batas otonomi agen` : 'Semua keputusan hari ini selesai'}</span></div>
            <Queue items={queue} />
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
              <li><span className="ag">{AGENT_NAMES[4]}</span><div>{(brief?.points[2]?.dealers ?? []).filter((d) => d.late_days).length} dealer dengan invoice lewat tempo; nada pengingat mengikuti pola bayar</div></li>
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

/** Agenda sales (mockup renderAgenda): busiest sales first, 3 dealers each; sales with nothing urgent fold into one line. */
function Agenda({ rows }: { rows: AgendaRow[] }) {
  const nav = useNavigate()
  const [openRow, setOpenRow] = useState('')
  const busy = rows.filter((r) => (r.items ?? []).length > 0).sort((a, b) => b.count - a.count)
  const idle = rows.filter((r) => (r.items ?? []).length === 0)
  const [shown, more] = useMore(busy, 6)
  return (
    <div className="agenda">
      {shown.map((r) => {
        const items = r.items ?? []
        const all = openRow === r.sales.key
        return (
          <div className="ag-row" key={r.sales.key}>
            <div className="ag-who">
              <span className="avatar">{r.sales.initials}</span>
              <div><b>{r.sales.name}</b><span>{r.sales.branch} · {r.dealers} dealer</span></div>
              <span className="ag-n" title="dealer perlu perhatian">{r.count}</span>
            </div>
            <ul>
              {(all ? items : items.slice(0, 3)).map((it) => (
                <li key={it.kind + it.dealer_id}>
                  <span className="dot" style={{ background: it.kind === 'due' ? 'var(--good)' : it.kind === 'drift' ? 'var(--warn)' : 'var(--bad)' }} />
                  <button className="ev" onClick={() => nav('/dealer/' + it.dealer_id)}>{it.short_name}</button>
                  <span className="ag-what">{it.kind === 'due' ? `jadwal ${dueLabel(it.due_in ?? 0)}` : it.kind === 'drift' ? `lewat ${it.late_days} hr` : 'tagih dulu'}</span>
                </li>
              ))}
              {items.length > 3 && <li><button className="ev-more" onClick={() => setOpenRow(all ? '' : r.sales.key)}>{all ? 'ringkas' : `+${items.length - 3} dealer lainnya`}</button></li>}
            </ul>
          </div>
        )
      })}
      {more}
      {idle.length > 0 && <p className="ag-idle">{idle.length} sales tanpa agenda mendesak: {idle.slice(0, 6).map((r) => r.sales.name.split(' ')[0]).join(', ')}{idle.length > 6 ? `, +${idle.length - 6}` : ''}</p>}
      {rows.length === 0 && <p className="ag-idle">Belum ada agenda hari ini.</p>}
    </div>
  )
}

/** "Update terakhir analisis": when the last cycle finished, its number, and how long ago (refreshed every minute). */
function LastUpdate() {
  const orch = useOrchStatus()
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(t)
  }, [])
  const c = orch.lastDone
  if (orch.running) return <div className="oc-last running"><Icon name="refresh" />Analisis sedang berjalan · {orch.stage}…</div>
  if (!c) return <div className="oc-last"><Icon name="cal" />Update terakhir analisis: <b>belum ada</b> · berikutnya {orch.next}</div>
  const at = c.finished_at ?? c.started_at
  const mins = Math.round((now - Date.parse(at)) / 60_000)
  const ago = mins < 1 ? 'baru saja' : mins < 60 ? `${mins} menit lalu` : mins < 24 * 60 ? `${Math.floor(mins / 60)} jam lalu` : ''
  const failed = c.status === 'failed'
  return (
    <div className={`oc-last${failed ? ' bad' : ''}`}>
      <Icon name={failed ? 'alert' : 'check'} />
      Update terakhir analisis: <b>{shortDate(at)} · {hhmm(at)} WIB</b>
      {ago && <span> ({ago})</span>}
      <span> · siklus #{(c.number ?? 0).toLocaleString('id-ID')} · {failed ? 'gagal' : c.status === 'partial' ? 'sebagian' : 'berhasil'}</span>
      <span> · berikutnya {orch.next}</span>
    </div>
  )
}
