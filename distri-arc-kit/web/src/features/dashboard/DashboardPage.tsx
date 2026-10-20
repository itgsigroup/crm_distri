import { Link } from 'react-router'
import { createContext, useContext, useState, type ReactNode } from 'react'
import { Icon } from '../../components/Icon'
import { useOrchStatus } from '../../app/orch'
import { useBrief, useCreditOverview, useKpi, useMe, useOrbit, useOrbitSummary, useStockCritical, useStockPush, useThreads, useTrend } from '../../app/queries'
import { fmtNum, fmtRp, hhmm, shortDate } from '../../lib/format'
import { KUAD } from '../../lib/i18n/id'
import type { BoardItem, Segment, Trend, TrendKey } from '../../api/types'
import { dueSoon } from '../orbit/filters'
import { DashboardStation } from './DashboardStation'
import { TrendChart } from './TrendChart'
import { TREND_BETTER, TREND_FMT, trendHeads } from './trend'
import './dashboard.css'

const segments: Segment[] = ['A', 'B', 'C', 'D', 'Baru']

// the history every card draws (12 months, or one month day by day)
const TrendCtx = createContext<{ data?: Trend; pending: boolean }>({ pending: true })

const MONTHS = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember']
const monthName = (ym: string) => `${MONTHS[Number(ym.slice(5, 7)) - 1]} ${ym.slice(0, 4)}`

function MetricTrend({ k, tone, unit }: { k?: TrendKey | 'stock'; tone?: string; unit: string }) {
  const { data, pending } = useContext(TrendCtx)
  if (!k) return null
  if (k === 'stock') return <span className="db-trend-na">Riwayat tidak tersedia — data stok hanya posisi saat ini</span>
  const values = data?.series[k]
  if (!values) return <span className="db-trend-na">{pending ? 'Memuat riwayat…' : 'Riwayat belum tersedia'}</span>
  return <TrendChart values={values} labels={data!.labels} heads={trendHeads(data!.mode, data!.labels, data!.dates, shortDate)} fmt={TREND_FMT[k]} tone={tone} unit={unit} better={TREND_BETTER[k]} />
}

function Metric({ label, value, detail, tone, to, trend, trendNote }: { label: string; value: ReactNode; detail: string; tone?: string; to?: string; trend?: TrendKey | 'stock'; trendNote?: string }) {
  const content = <><span className="db-metric-label">{label}</span><strong className="num">{value}</strong><span className="db-metric-detail">{detail}</span>
    <MetricTrend k={trend} tone={tone} unit={trendNote ?? label} />{trendNote && trend && trend !== 'stock' && <span className="db-trend-note">{trendNote}</span>}
    {to && <Icon name="chev" className="i db-metric-arrow" />}</>
  return to ? <Link className={`db-metric ${tone ?? ''}`} to={to}>{content}</Link> : <div className={`db-metric ${tone ?? ''}`}>{content}</div>
}

/** "12 bulan | Per hari" and, per day, which month. */
function TrendBar({ mode, setMode, month, setMonth, months }: { mode: 'months' | 'days'; setMode: (m: 'months' | 'days') => void; month: string; setMonth: (m: string) => void; months: string[] }) {
  return <div className="db-trend-bar">
    <div><b>Riwayat di setiap kartu</b><span>{mode === 'months' ? '12 bulan terakhir · satu titik per bulan' : `${monthName(month)} · tanggal 1 sampai akhir bulan`}</span></div>
    <div className="db-trend-ctl">
      <div className="seg" role="tablist" aria-label="Rentang riwayat">
        <button role="tab" aria-selected={mode === 'months'} className={mode === 'months' ? 'is-active' : ''} onClick={() => setMode('months')}>12 bulan</button>
        <button role="tab" aria-selected={mode === 'days'} className={mode === 'days' ? 'is-active' : ''} onClick={() => setMode('days')}>Per hari</button>
      </div>
      {mode === 'days' && months.length > 0 && <select className="db-trend-month" aria-label="Bulan" value={month} onChange={(e) => setMonth(e.target.value)}>
        {months.map((m) => <option key={m} value={m}>{monthName(m)}</option>)}
      </select>}
    </div>
  </div>
}

function CreditMetrics() {
  const { data, isPending, isError } = useCreditOverview()
  const value = (n: number | undefined) => isPending || isError || n == null ? '—' : fmtRp(n)
  return <>
    <Metric label="Piutang terbuka" value={value(data?.receivable)} detail={isError ? 'Data kredit gagal dimuat' : `${fmtNum(data?.open_dealers ?? 0)} dealer · ${fmtNum(data?.open_invoices ?? 0)} invoice`} to="/kredit" trend="receivable" />
    <Metric label="Lewat tempo" value={value(data?.overdue)} detail={isError ? 'Data kredit gagal dimuat' : `${fmtNum(data?.overdue_dealers ?? 0)} dealer`} tone="bad" to="/kredit" trend="overdue" />
    <Metric label="Kas masuk · prediksi 30 hari" value={value(data?.forecast_30)} detail="Berdasarkan pola bayar dealer" to="/kredit" trend="cash_in" trendNote="Grafik: kas masuk aktual" />
  </>
}

function StockCritical() {
  const { data, isPending, isError } = useStockCritical()
  return <div className="db-signal">
    <span className="db-signal-icon bad"><Icon name="box" /></span>
    <div><b>Stok kritis</b><span>{isError ? 'Data stok gagal dimuat' : isPending ? 'Memuat…' : `${fmtNum(data?.length ?? 0)} SKU berisiko habis sebelum siklus order berikutnya`}</span></div>
    <Link to="/stok" aria-label="Buka stok kritis"><Icon name="chev" /></Link>
  </div>
}

function ChatSignal() {
  const { data, isPending, isError } = useThreads('all', '')
  const unread = data?.reduce((sum, item) => sum + item.unread, 0) ?? 0
  return <div className="db-signal">
    <span className="db-signal-icon accent"><Icon name="chat" /></span>
    <div><b>Chat belum dibaca</b><span>{isError ? 'Data chat gagal dimuat' : isPending ? 'Memuat…' : `${fmtNum(unread)} pesan memerlukan perhatian`}</span></div>
    <Link to="/chat" aria-label="Buka chat"><Icon name="chev" /></Link>
  </div>
}

function RiskDealers({ dealers, canOpen, canOrbit, pending, error }: { dealers: BoardItem[]; canOpen: boolean; canOrbit: boolean; pending: boolean; error: boolean }) {
  const risky = dealers.filter((d) => d.metrics.status === 'At risk' || d.metrics.status === 'Churn')
    .sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln).slice(0, 5)
  return <div className="card db-panel">
    <div className="card-h"><h2>Dealer perlu perhatian</h2><span className="meta">Urut omzet bulanan terbesar</span></div>
    {pending || error ? <p className="db-empty">{error ? 'Data dealer belum tersedia.' : 'Memuat dealer…'}</p> : risky.length ? <div className="db-dealers">{risky.map((d) => <div className="db-dealer" key={d.id}>
      <span className={`db-status-dot ${d.metrics.status === 'Churn' ? 'bad' : 'warn'}`} />
      <div>{canOpen ? <Link to={`/dealer/${d.id}`}>{d.name}</Link> : <b>{d.name}</b>}<small>{d.city} · {d.owner.name} · {d.metrics.status}</small></div>
      <strong className="num">{fmtRp(d.metrics.omzet_bln)}</strong>
    </div>)}</div> : <p className="db-empty">Tidak ada dealer berstatus At risk atau Churn.</p>}
    {canOrbit && <Link className="db-panel-link" to="/orbit">Lihat semua dealer <Icon name="chev" /></Link>}
  </div>
}

export function DashboardPage() {
  const { data: me } = useMe()
  const { data: board, isPending: boardPending, isError: boardError } = useOrbit()
  const { data: orbitSummary } = useOrbitSummary()
  const { data: kpi, isPending: kpiPending, isError: kpiError } = useKpi()
  const { data: push, isPending: pushPending, isError: pushError } = useStockPush()
  const { data: brief } = useBrief()
  const orch = useOrchStatus()
  const [trendMode, setTrendMode] = useState<'months' | 'days'>('months')
  const [trendMonth, setTrendMonth] = useState('')
  const trend = useTrend(trendMode, trendMode === 'days' ? trendMonth || undefined : undefined)
  const trendMonths = trend.data?.months ?? []
  // actual omzet by month (invoices of the month), not the dealers' 6-month average — the same footing as Sales Arena
  const monthly = useTrend('months')
  const om = monthly.data?.series.omzet ?? []
  const lastIdx = om.reduce<number>((acc, v, i) => (v != null ? i : acc), -1)
  const thisMonth = lastIdx >= 0 ? om[lastIdx] : null
  const prevMonth = lastIdx >= 1 ? om[lastIdx - 1] : null
  const prevLabel = lastIdx >= 1 ? monthly.data!.labels[lastIdx - 1] : ''
  const src = kpi?.data
  const srcText = src ? `${src.label}${src.synced_at ? ` · sinkron ${shortDate(src.synced_at)} ${hhmm(src.synced_at)} WIB` : ''}` : ''
  const can = (screen: string) => me?.screens.includes(screen) ?? false
  const dealers = board ?? []
  const active = dealers.filter((d) => d.metrics.segment !== 'Prospek')
  const omzet = active.reduce((sum, d) => sum + d.metrics.omzet_bln, 0)
  const atRisk = active.filter((d) => d.metrics.status === 'At risk' || d.metrics.status === 'Churn').length
  const due = active.filter(dueSoon).length
  const segmentRows = segments.map((segment) => {
    const items = active.filter((d) => d.metrics.segment === segment)
    return { segment, count: items.length, revenue: items.reduce((sum, d) => sum + d.metrics.omzet_bln, 0) }
  })
  const linkItems = [
    ['today', '/', 'sun', 'Pusat kendali', 'Keputusan dan pekerjaan hari ini'],
    ['orch', '/orchestrator', 'spark', 'Orchestrator', 'Siklus analisis dan agen AI'],
    ['orbit', '/orbit', 'target', 'Orbit', 'Posisi dan jadwal dealer'],
    ['kuad', '/orbit/segmen', 'chart', 'Segmen', 'Sebaran frekuensi dan nilai order'],
    ['net', '/orbit/relasi', 'net', 'Peta relasi', 'Hubungan sales dan dealer'],
    ['dealer', '/dealer', 'building', 'Dealer', 'Profil, order, dan tindak lanjut'],
    ['stock', '/stok', 'box', 'Push stok', 'Stok menua dan kritis'],
    ['ar', '/kredit', 'cash', 'Kredit · kas', 'Piutang, limit, dan prediksi kas'],
    ['chat', '/chat', 'chat', 'Chat', 'Percakapan dan pesan masuk'],
    ['users', '/pengguna', 'people', 'Pengguna', 'Akun dan penugasan'],
    ['roles', '/peran', 'shield', 'Peran & akses', 'Hak akses dan keputusan'],
    ['branches', '/cabang', 'building', 'Cabang', 'Master wilayah kerja'],
    ['mcp', '/claude', 'spark', 'MCP Claude', 'Koneksi asisten AI'],
    ['conn', '/pengaturan', 'gear', 'Pengaturan', 'Koneksi data dan kebijakan'],
    ['konsep', '/panduan', 'doc', 'Panduan', 'Cara membaca data dan fitur'],
  ] as const

  return <div className="db-page">
    <div className="db-hero">
      <div><span className="db-eyebrow">GSI ORBIT · RINGKASAN BISNIS</span><h2>Kondisi bisnis dalam satu layar</h2><p>Angka terbaru dari dealer, order, kredit, dan stok. Buka setiap bagian untuk melihat detailnya.</p></div>
      <div className="db-hero-meta"><span>Dealer dalam cakupan Anda</span><b>{board?.[0]?.metrics.as_of ? `Data dealer ${shortDate(board[0].metrics.as_of)}` : 'Data terbaru'}</b>{srcText && <small title={src?.basis}>Sumber {srcText}</small>}{brief?.generated_at && <small>Ringkasan AI diperbarui {hhmm(brief.generated_at)} WIB</small>}</div>
    </div>

    <DashboardStation board={dealers} kpi={kpi} can={can} />

    {(boardError || kpiError || pushError) && <div className="db-error" role="alert">Sebagian data gagal dimuat. Muat ulang halaman untuk mencoba lagi.</div>}

    <TrendBar mode={trendMode} setMode={setTrendMode} month={trendMonth || trendMonths[0] || ''} setMonth={setTrendMonth} months={trendMonths} />
    <TrendCtx.Provider value={{ data: trend.data, pending: trend.isPending }}>
    <div className="db-metrics">
      <Metric label="Dealer di Orbit" value={boardPending || boardError ? '—' : fmtNum(active.length)} detail={boardPending ? 'Memuat data dealer…' : `${fmtNum(orbitSummary?.prospects ?? 0)} prospek di luar orbit`} to={can('dealer') ? '/dealer' : undefined} trend="dealers" />
      <Metric label={`Omzet aktual · ${prevLabel || 'bulan lalu'}`} value={monthly.isPending || prevMonth == null ? '—' : fmtRp(prevMonth)}
        detail={`Bulan ini s.d. hari ini ${thisMonth == null ? '—' : fmtRp(thisMonth)} · rata-rata dealer 6 bln ${boardPending || boardError ? '—' : fmtRp(omzet)}/bln${srcText ? ` · ${srcText}` : ''}`}
        to={can('kuad') ? '/orbit/segmen' : undefined} trend="omzet" trendNote={trendMode === 'months' ? 'Grafik: omzet aktual per bulan' : 'Grafik: omzet aktual per hari'} />
      <Metric label="Order tepat jadwal" value={kpiPending || kpiError ? '—' : `${kpi?.on_schedule_pct ?? 0}%`} detail={kpi ? `Target ${kpi.targets.on_schedule_pct}%` : 'Dari KPI order'} tone={kpi && kpi.on_schedule_pct < kpi.targets.on_schedule_pct ? 'warn' : undefined} to={can('today') ? '/' : undefined} trend="on_schedule" />
      <Metric label="Dealer At risk / Churn" value={boardPending || boardError ? '—' : fmtNum(atRisk)} detail="Perlu tindak lanjut" tone="bad" to={can('orbit') ? '/orbit' : undefined} trend="at_risk" />
      <Metric label="Jadwal order · 14 hari" value={boardPending || boardError ? '—' : fmtNum(due)} detail="Dealer mendekati jadwal order" to={can('orbit') ? '/orbit?jadwal=minggu' : undefined} trend="due" />
      <Metric label="Stok menua untuk didorong" value={pushPending || pushError ? '—' : fmtNum(push?.length ?? 0)} detail={pushPending ? 'Memuat data stok…' : `${fmtRp((push ?? []).reduce((sum, item) => sum + item.value, 0))} nilai stok`} tone="warn" to={can('stock') ? '/stok' : undefined} trend="stock" />
      <Metric label="DSO · order ke bayar" value={kpiPending || kpiError ? '—' : `${kpi?.dso_days ?? 0} hari`} detail={kpi ? `Target ≤ ${kpi.targets.dso_days} hari` : 'Rata-rata waktu pembayaran'} tone={kpi && kpi.dso_days > kpi.targets.dso_days ? 'warn' : undefined} to={can('ar') ? '/kredit' : undefined} trend="dso" />
      <Metric label="Perputaran stok" value={kpiPending || kpiError ? '—' : `${kpi?.stock_turn_days ?? 0} hari`} detail={kpi ? `Target ≤ ${kpi.targets.stock_turn_days} hari` : 'Rata-rata waktu stok berputar'} tone={kpi && kpi.stock_turn_days > kpi.targets.stock_turn_days ? 'warn' : undefined} to={can('stock') ? '/stok' : undefined} trend="stock" />
      <Metric label="Limit dealer tipis" value={kpiPending || kpiError ? '—' : fmtNum(kpi?.tight_count ?? 0)} detail="Berpotensi menahan order" tone="warn" to={can('ar') ? '/kredit' : undefined} trend="tight" />
      {can('ar') && <CreditMetrics />}
    </div>
    </TrendCtx.Provider>

    <div className="db-main">
      <div className="card db-panel">
        <div className="card-h"><h2>Komposisi segmen dealer</h2><span className="meta">Porsi dari omzet bulanan</span></div>
        {boardPending ? <p className="db-empty">Memuat segmen…</p> : boardError ? <p className="db-empty">Data segmen belum tersedia.</p> : <div className="db-segments">{segmentRows.map(({ segment, count, revenue }) => <div className="db-segment" key={segment}>
          <div className="db-segment-head"><b>{KUAD[segment].n} <small>{KUAD[segment].nick}</small></b><span>{fmtNum(count)} dealer · {fmtRp(revenue)}</span></div>
          <div className="db-bar"><i className={`segment-${segment.toLowerCase()}`} style={{ width: `${omzet ? Math.max(revenue ? 1 : 0, Math.round(revenue / omzet * 100)) : 0}%` }} /></div>
          <small>{omzet ? Math.round(revenue / omzet * 100) : 0}% omzet</small>
        </div>)}</div>}
        {can('kuad') && <Link className="db-panel-link" to="/orbit/segmen">Buka analisis segmen <Icon name="chev" /></Link>}
      </div>
      <RiskDealers dealers={dealers} canOpen={can('dealer')} canOrbit={can('orbit')} pending={boardPending} error={boardError} />
    </div>

    <div className="card db-panel">
      <div className="card-h"><h2>Sinyal operasional</h2><span className="meta">Hal yang mungkin perlu ditindaklanjuti</span></div>
      <div className="db-signals">
        <div className="db-signal"><span className="db-signal-icon accent"><Icon name="spark" /></span><div><b>Keputusan menunggu</b><span>{fmtNum(orch.pending)} keputusan · {orch.running ? 'analisis sedang berjalan' : orch.run ? `siklus #${fmtNum(orch.run)}` : 'belum ada siklus'}</span></div>{can('today') && <Link to="/" aria-label="Buka keputusan"><Icon name="chev" /></Link>}</div>
        <div className="db-signal"><span className="db-signal-icon warn"><Icon name="target" /></span><div><b>Dealer lewat jadwal</b><span>{kpiPending || kpiError ? 'Data belum tersedia' : `${fmtNum(kpi?.drift_count ?? 0)} dealer melewati siklus order`}</span></div>{can('orbit') && <Link to="/orbit" aria-label="Buka orbit"><Icon name="chev" /></Link>}</div>
        {can('stock') && <StockCritical />}
        {can('chat') && <ChatSignal />}
      </div>
    </div>

    <div className="card db-panel"><div className="card-h"><h2>Jelajahi fitur</h2><span className="meta">Sesuai akses akun Anda</span></div><div className="db-links">{linkItems.filter(([screen]) => can(screen)).map(([screen, to, icon, title, description]) => <Link key={screen} to={to} className="db-link"><span><Icon name={icon} /></span><div><b>{title}</b><small>{description}</small></div><Icon name="chev" className="i db-link-arrow" /></Link>)}</div></div>
  </div>
}
