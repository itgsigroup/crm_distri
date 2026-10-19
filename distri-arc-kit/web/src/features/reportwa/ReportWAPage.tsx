import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import type { WAReport, WAReportDay, WAReportHang, WAReportNew, WAReportSales } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ColumnTable, type Column } from '../../components/DataTable'
import { CardH, Pill } from '../../components/ui'
import { SalesBanner, SalesPicker, useSalesPage } from '../../components/SalesPicker'
import { fmtNum, fmtRp, hhmm, shortDate } from '../../lib/format'
import { useWAReport } from '../../app/queries'
import { waitLabel } from '../wa/wa'

// Report WhatsApp (ADR 0050): per sales — response speed, SLA, hanging chats, broken "cek dulu" promises, new numbers,
// order requests won — scored and ranked, with good / bad feedback; for the director the team picture, the daily trend
// and the hours customers chat. Every number is measured from the chats (no AI).

const PERIODS = [[1, 'Hari ini'], [7, '7 hari'], [30, '30 hari']] as const
const DAYS = ['Min', 'Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab']
const HOURS = Array.from({ length: 17 }, (_, i) => i + 6) // 06–22 WIB

const pct = (a: number, b: number) => (b ? Math.round((a * 100) / b) : 0)
const mins = (m: number | null | undefined) => (m == null ? '—' : waitLabel(m))
const nul = (m: number | null | undefined) => m ?? Number.MAX_SAFE_INTEGER // unknown times sort last
const tone = (score: number) => (score >= 80 ? 'good' : score >= 60 ? 'warn' : 'bad')

export function ReportWAPage() {
  const page = useSalesPage()
  const [days, setDays] = useState(7)
  const { data: rep, isPending, isError, error } = useWAReport(days, page.sales)
  return (
    <>
      <SalesPicker page={page} />
      <SalesBanner page={page} />
      <div className="rw-bar">
        <div className="seg" role="radiogroup" aria-label="Periode">
          {PERIODS.map(([d, l]) => <button key={d} role="radio" aria-checked={days === d} className={days === d ? 'is-active' : ''} onClick={() => setDays(d)}>{l}</button>)}
        </div>
        {rep && <span className="meta">{shortDate(rep.since)} – {shortDate(rep.until)} · {hhmm(rep.until)} WIB · target balas ≤ {waitLabel(rep.sla_min)} · gantung ≥ {waitLabel(rep.hang_min)} (jam kerja)</span>}
      </div>
      {isPending && <div className="card"><p className="sg-hint">Menghitung report dari chat WhatsApp…</p></div>}
      {isError && <div className="card"><p className="sg-hint">Report gagal dimuat: {(error as Error).message}</p></div>}
      {rep && <Report rep={rep} />}
    </>
  )
}

function Report({ rep }: { rep: WAReport }) {
  const t = rep.team
  const ranked = rep.sales.filter((s) => s.requests > 0)
  return (
    <div className="stack">
      <div className="rw-kpis">
        <Kpi label="Chat masuk" value={fmtNum(t.inbound)} sub={`${fmtNum(t.threads)} percakapan · ${fmtNum(t.dealers)} dealer`} />
        <Kpi label="Dibalas ≤ target" value={onTimeText(t)} sub={`median ${mins(t.reply_median_min)} · p90 ${mins(t.reply_p90_min)}`} tone={t.answered_work ? (onTime(t) >= 80 ? 'good' : 'warn') : undefined} />
        <Kpi label="Permintaan dijawab" value={`${pct(t.answered, t.requests)}%`} sub={`${fmtNum(t.answered)} dari ${fmtNum(t.requests)}`} />
        <Kpi label="Chat gantung" value={fmtNum(t.hanging + t.promises)} sub={`${t.hanging} menunggu · ${t.promises} janji "cek dulu"`} tone={t.hanging + t.promises ? 'bad' : 'good'} />
        <Kpi label="Nomor baru" value={fmtNum(t.new_numbers)} sub={`balas pertama ${mins(t.new_reply_median_min)}`} />
        <Kpi label="Order dari chat" value={`${t.order_won}/${t.order_asks}`} sub={t.order_won ? `jadi SO · ${fmtRp(t.order_value)}` : `jadi SO dalam ${rep.win_days} hari`} />
      </div>

      <div className="rw-grid">
        <div className="card rw-insights">
          <CardH title="Ringkasan untuk direktur & manager" meta="dihitung dari chat, tanpa AI" />
          <ul>{rep.insights.map((x) => <li key={x}><Icon name="spark" />{x}</li>)}</ul>
        </div>
        <Winner s={ranked[0]} />
      </div>

      <div className="card">
        <CardH title="Peringkat sales" meta="skor 0–100: dijawab 35 · tepat waktu 30 · kecepatan 20 · tanpa gantung 15" />
        <RankTable rows={rep.sales} sla={rep.sla_min} />
      </div>

      <div className="card">
        <CardH title="Feedback per sales" meta="yang sudah baik dan yang perlu diperbaiki — dari angka periode ini" />
        {rep.sales.length === 0 ? <p className="sg-hint">Belum ada chat pelanggan di periode ini.</p> : (
          <div className="rw-fb">
            {rep.sales.map((s) => <Feedback key={s.name} s={s} />)}
          </div>
        )}
      </div>

      <div className="rw-grid">
        <div className="card">
          <CardH title="Tren harian" meta="chat pelanggan per hari · arahkan ke batang untuk median balas" />
          <Daily days={rep.daily} />
        </div>
        <div className="card">
          <CardH title="Jam ramai pelanggan" meta="chat masuk per hari × jam (WIB) — atur jadwal jaga sales" />
          <Heat heat={rep.heat} />
        </div>
      </div>

      <div className="card" id="hanging">
        <CardH title="Chat gantung" meta={`pelanggan menunggu ≥ ${waitLabel(rep.hang_min)} jam kerja, atau dijanjikan "cek dulu" tanpa kabar`} />
        <HangTable rows={rep.hanging} />
      </div>

      <div className="card">
        <CardH title="Nomor baru" meta="calon pelanggan yang chat pertama kali di periode ini" />
        <NewTable rows={rep.new_numbers} />
      </div>
    </div>
  )
}

/** % of the requests asked in working hours answered within the target (outside working hours is not measured). */
const onTime = (s: WAReportSales) => pct(s.within_sla, s.answered_work)
const onTimeText = (s: WAReportSales) => (s.answered_work ? `${onTime(s)}%` : '—')

/** Winning sales: the best score of the period, when it is a good one (≥ 60 with at least 3 chats). */
function Winner({ s }: { s?: WAReportSales }) {
  const won = s && s.score >= 60 && s.requests >= 3
  return (
    <div className={`card rw-win ${won ? '' : 'none'}`}>
      <span className="rw-medal">{won ? <Icon name="check" /> : '–'}</span>
      {won ? (
        <div><small>Winning sales</small><b>{s.name}</b><span>skor {s.score} · {onTime(s)}% dibalas tepat waktu · median {mins(s.reply_median_min)} · {s.order_won} order dari chat</span></div>
      ) : (
        <div><small>Winning sales</small><b>Belum ada</b><span>butuh skor ≥ 60 dan minimal 3 chat di periode ini{s ? ` — tertinggi ${s.name}, skor ${s.score}` : ''}</span></div>
      )}
    </div>
  )
}

function Kpi({ label, value, sub, tone }: { label: string; value: string; sub: string; tone?: string }) {
  return (
    <div className="rw-kpi">
      <small>{label}</small>
      <b className={tone ? `t-${tone}` : undefined}>{value}</b>
      <span>{sub}</span>
    </div>
  )
}

function RankTable({ rows, sla }: { rows: WAReportSales[]; sla: number }) {
  const cols: Column<WAReportSales>[] = [
    { key: 'rank', label: '#', cell: (s) => (s.rank ? <span className={`rw-rank r${Math.min(s.rank, 4)}`}>{s.rank}</span> : '—'), sort: (a, b) => (a.rank || 999) - (b.rank || 999) },
    { key: 'sales', label: 'Sales', cell: (s) => <b>{s.name}</b>, sort: (a, b) => a.name.localeCompare(b.name, 'id') },
    { key: 'skor', label: 'Skor', cell: (s) => <span className="rw-score"><i className={`t-${tone(s.score)}`} style={{ width: `${s.score}%` }} /><b>{s.score}</b></span>, sort: (a, b) => a.score - b.score, first: 'desc' },
    { key: 'chat', label: 'Chat', cell: (s) => fmtNum(s.requests), sort: (a, b) => a.requests - b.requests, first: 'desc', right: true },
    { key: 'sla', label: `≤ ${waitLabel(sla)}`, cell: (s) => onTimeText(s), sort: (a, b) => (a.answered_work ? onTime(a) : -1) - (b.answered_work ? onTime(b) : -1), first: 'desc', right: true },
    { key: 'median', label: 'Median', cell: (s) => mins(s.reply_median_min), sort: (a, b) => nul(a.reply_median_min) - nul(b.reply_median_min), right: true },
    { key: 'gantung', label: 'Gantung', cell: (s) => (s.hanging + s.promises ? <span className="t-bad">{s.hanging + s.promises}</span> : '0'), sort: (a, b) => a.hanging + a.promises - (b.hanging + b.promises), first: 'desc', right: true },
    { key: 'baru', label: 'Nomor baru', cell: (s) => fmtNum(s.new_numbers), sort: (a, b) => a.new_numbers - b.new_numbers, first: 'desc', right: true },
    { key: 'order', label: 'Order → SO', cell: (s) => (s.order_asks ? `${s.order_won}/${s.order_asks}` : '—'), sort: (a, b) => a.order_won - b.order_won, first: 'desc', right: true },
  ]
  return <ColumnTable rows={rows} columns={cols} rowKey={(s) => s.name} text={(s) => s.name} noun="sales" placeholder="Cari sales…" initial={{ column: 'rank', dir: 'asc' }} empty="Belum ada chat pelanggan di periode ini." />
}

function Feedback({ s }: { s: WAReportSales }) {
  return (
    <div className="rw-fb-card">
      <div className="rw-fb-h">
        {s.rank > 0 && <span className={`rw-rank r${Math.min(s.rank, 4)}`}>{s.rank}</span>}
        <b>{s.name}</b>
        <Pill tone={tone(s.score)}>skor {s.score}</Pill>
      </div>
      <div className="rw-fb-nums">
        <span><b>{fmtNum(s.requests)}</b> chat</span>
        <span>median <b>{mins(s.reply_median_min)}</b></span>
        <span><b>{s.new_numbers}</b> nomor baru</span>
        {s.waiting > 0 && <span><b>{s.waiting}</b> menunggu</span>}
      </div>
      {s.good.length > 0 && <ul className="rw-good">{s.good.map((x) => <li key={x}><Icon name="check" />{x}</li>)}</ul>}
      {s.bad.length > 0 && <ul className="rw-bad">{s.bad.map((x) => <li key={x}><Icon name="alert" />{x}</li>)}</ul>}
    </div>
  )
}

/** Customer messages per day: one series, bars from the baseline; the tooltip adds our replies and the median. */
function Daily({ days }: { days: WAReportDay[] }) {
  const [hover, setHover] = useState<number | null>(null)
  if (days.length === 0) return <p className="sg-hint">Belum ada data.</p>
  const max = Math.max(1, ...days.map((d) => d.inbound))
  const W = 600, H = 170, pad = 22, bw = Math.max(4, Math.min(36, (W - pad) / days.length - 4))
  const step = (W - pad) / days.length
  const h = hover != null ? days[hover] : null
  return (
    <div className="rw-daily">
      <svg viewBox={`0 0 ${W} ${H + 22}`} role="img" aria-label="Chat pelanggan per hari">
        {[0.5, 1].map((f) => <line key={f} x1={pad} x2={W} y1={H - (H - 10) * f} y2={H - (H - 10) * f} className="rw-grid-l" />)}
        <text x={0} y={14} className="rw-ax">{max}</text>
        {days.map((d, i) => {
          const bh = Math.max(d.inbound ? 3 : 0, ((H - 10) * d.inbound) / max)
          const x = pad + i * step + (step - bw) / 2
          return (
            <g key={d.date} onMouseEnter={() => setHover(i)} onMouseLeave={() => setHover(null)}>
              <rect x={pad + i * step} y={0} width={step} height={H + 22} fill="transparent" />
              <rect x={x} y={H - bh} width={bw} height={bh} rx={Math.min(4, bw / 2)} className={`rw-bar-m${hover === i ? ' on' : ''}`} />
              {(days.length <= 10 || i % Math.ceil(days.length / 8) === 0) && <text x={x + bw / 2} y={H + 16} textAnchor="middle" className="rw-ax">{d.date.slice(8)}/{d.date.slice(5, 7)}</text>}
            </g>
          )
        })}
      </svg>
      <div className="rw-tip" aria-live="polite">
        {h ? <><b>{shortDate(h.date)}</b> · {fmtNum(h.inbound)} chat masuk · {fmtNum(h.outbound)} balasan · median balas {mins(h.reply_median_min)}</> : <span className="muted">Arahkan ke batang untuk detail hari itu.</span>}
      </div>
    </div>
  )
}

/** Customer messages by weekday × hour: one hue, light → dark. */
function Heat({ heat }: { heat: number[][] }) {
  const max = Math.max(1, ...heat.flat())
  const order = [1, 2, 3, 4, 5, 6, 0] // Monday first
  return (
    <div className="rw-heat" role="table" aria-label="Chat masuk per hari dan jam">
      <div className="rw-heat-row rw-heat-head" role="row"><span />{HOURS.map((h) => <span key={h} role="columnheader">{h % 2 === 0 ? String(h).padStart(2, '0') : ''}</span>)}</div>
      {order.map((d) => (
        <div className="rw-heat-row" role="row" key={d}>
          <span role="rowheader">{DAYS[d]}</span>
          {HOURS.map((h) => {
            const n = heat[d]?.[h] ?? 0
            return <span key={h} role="cell" className="rw-cell" title={`${DAYS[d]} ${String(h).padStart(2, '0')}.00 · ${n} chat`} style={{ background: n ? `color-mix(in srgb, var(--accent) ${15 + Math.round((85 * n) / max)}%, var(--surface-2))` : undefined }} />
          })}
        </div>
      ))}
      <div className="rw-heat-legend"><span>sepi</span><i /><span>ramai ({max} chat/jam)</span></div>
    </div>
  )
}

function HangTable({ rows }: { rows: WAReportHang[] }) {
  const nav = useNavigate()
  const cols: Column<WAReportHang>[] = [
    { key: 'wait', label: 'Menunggu', cell: (h) => <Pill tone={h.wait_min >= 8 * 60 ? 'bad' : 'warn'}>{waitLabel(h.wait_min)}</Pill>, sort: (a, b) => a.wait_min - b.wait_min, first: 'desc' },
    { key: 'who', label: 'Pelanggan', cell: (h) => <>{h.dealer_slug ? <Link className="ev" to={'/dealer/' + h.dealer_slug}>{h.title}</Link> : <b>{h.title}</b>}<span className="cc-sub">{h.kind === 'new' ? 'nomor baru' : h.kind === 'group' ? 'grup' : 'dealer'} · sejak {shortDate(h.since)} {hhmm(h.since)}</span></>, sort: (a, b) => a.title.localeCompare(b.title, 'id') },
    { key: 'sales', label: 'Sales', cell: (h) => h.sales, sort: (a, b) => a.sales.localeCompare(b.sales, 'id') },
    { key: 'why', label: 'Jenis', cell: (h) => (h.why === 'janji' ? <Pill tone="warn">janji "cek dulu"</Pill> : <Pill tone="bad">belum dibalas</Pill>), sort: (a, b) => a.why.localeCompare(b.why) },
    { key: 'text', label: 'Pesan terakhir pelanggan', cell: (h) => <q className="rw-q">{h.last_text || '—'}</q> },
    { key: 'go', label: '', cell: (h) => <button className="btn ghost cc-ico" title="Buka chat" aria-label="Buka chat" onClick={() => nav('/chat/' + h.thread_id)}><Icon name="chat" /></button> },
  ]
  return <ColumnTable rows={rows} columns={cols} rowKey={(h) => h.thread_id} text={(h) => `${h.title} ${h.sales} ${h.last_text} ${h.why}`} noun="chat" placeholder="Cari pelanggan, sales, pesan…" initial={{ column: 'wait', dir: 'desc' }} empty="Tidak ada chat gantung. 👍" />
}

function NewTable({ rows }: { rows: WAReportNew[] }) {
  const nav = useNavigate()
  const cols: Column<WAReportNew>[] = [
    { key: 'first', label: 'Chat pertama', cell: (n) => <span className="num">{shortDate(n.first_at)} · {hhmm(n.first_at)}</span>, sort: (a, b) => Date.parse(a.first_at) - Date.parse(b.first_at), first: 'desc', className: 'nowrap' },
    { key: 'who', label: 'Nomor', cell: (n) => <b>{n.title}</b>, sort: (a, b) => a.title.localeCompare(b.title, 'id') },
    { key: 'sales', label: 'Sales', cell: (n) => n.sales, sort: (a, b) => a.sales.localeCompare(b.sales, 'id') },
    { key: 'reply', label: 'Balas pertama', cell: (n) => (n.first_reply_min == null ? <Pill tone="bad">belum dibalas</Pill> : waitLabel(n.first_reply_min)), sort: (a, b) => nul(a.first_reply_min) - nul(b.first_reply_min), right: true },
    { key: 'pesan', label: 'Pesan', cell: (n) => fmtNum(n.inbound), sort: (a, b) => a.inbound - b.inbound, first: 'desc', right: true },
    { key: 'text', label: 'Pesan terakhir', cell: (n) => <q className="rw-q">{n.last_text || '—'}</q> },
    { key: 'go', label: '', cell: (n) => <button className="btn ghost cc-ico" title="Buka chat" aria-label="Buka chat" onClick={() => nav('/chat/' + n.thread_id)}><Icon name="chat" /></button> },
  ]
  return <ColumnTable rows={rows} columns={cols} rowKey={(n) => n.thread_id} text={(n) => `${n.title} ${n.sales} ${n.last_text}`} noun="nomor" placeholder="Cari nomor, sales, pesan…" initial={{ column: 'first', dir: 'desc' }} empty="Tidak ada nomor baru di periode ini." />
}
