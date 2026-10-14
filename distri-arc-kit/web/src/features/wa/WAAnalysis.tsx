import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import type { WAAnalysis, WAHistoryRow, WAInsight } from '../../api/types'
import { Icon } from '../../components/Icon'
import { CardH, Pill } from '../../components/ui'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { fmtNum, hhmm, shortDate } from '../../lib/format'
import { useMe, useWAAnalysis } from '../../app/queries'
import { WA_COMPARE, waFilter, waFirstDir, waText, waTie, waitLabel, who, type WAColumn, type WAFilter } from './wa'

// Analisa WhatsApp (ADR 0026): cards inside Pusat kendali and Orchestrator.

/** WhatsApp analysis quotes customers: only for users with the chat page. */
export function useCanWA() {
  const { data: me } = useMe()
  return !!me?.screens.includes('chat')
}

/** "5 menit lalu", "2 jam lalu"; '' after a day (the date says enough). */
function ago(at: string, now: number): string {
  const mins = Math.round((now - Date.parse(at)) / 60_000)
  if (mins < 1) return 'baru saja'
  if (mins < 60) return `${mins} menit lalu`
  if (mins < 24 * 60) return `${Math.floor(mins / 60)} jam lalu`
  return ''
}

/** "Analisa WhatsApp terakhir: 12 Okt · 12.00 WIB (5 menit lalu) · siklus #98 · 34 chat" — when the Orchestrator last
 * read the chats. Older than two hours (a cycle missed) turns amber. Hidden for users without the chat page. */
export function WALastRead({ sales, compact = false }: { sales?: string; compact?: boolean }) {
  const canWA = useCanWA()
  const { data } = useWAAnalysis(sales, canWA)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(t)
  }, [])
  if (!canWA || !data) return null
  if (!data.generated_at) {
    return <div className={`wa-last${compact ? ' compact' : ''}`}><Icon name="chat" />Analisa WhatsApp terakhir: <b>belum ada</b> · dibaca di siklus berikutnya</div>
  }
  const at = data.generated_at
  const stale = now - Date.parse(at) > 2 * 60 * 60_000
  const a = ago(at, now)
  return (
    <div className={`wa-last${stale ? ' stale' : ''}${compact ? ' compact' : ''}`} title={stale ? 'Lebih dari 2 jam sejak chat terakhir dibaca — cek Riwayat analisis' : undefined}>
      <Icon name="chat" />Analisa WhatsApp terakhir: <b>{shortDate(at)} · {hhmm(at)} WIB</b>
      {a && <span> ({a})</span>}
      {!compact && <span> · siklus #{fmtNum(data.cycle_number ?? 0)} · {fmtNum(data.totals.threads)} chat</span>}
    </div>
  )
}

const replyText = (m: number | null) => (m == null ? '—' : waitLabel(m))

/** Counts, requests and the AI overview. */
export function WAOverviewCard({ data, scoped }: { data: WAAnalysis | undefined; scoped: boolean }) {
  const t = data?.totals
  const due = data ? waitLabel(data.reply_due_min) : '2 jam'
  return (
    <div className="card">
      <CardH title="Analisa WhatsApp" ai="Claude · MCP" meta={data?.cycle_number ? `siklus #${fmtNum(data.cycle_number)} · ${data.generated_at ? `${shortDate(data.generated_at)} · ${hhmm(data.generated_at)}` : ''} · ${data.window_days} hari terakhir` : 'belum ada siklus'} />
      {!data?.cycle_number ? (
        <p className="sg-hint">Orchestrator membaca chat WhatsApp di setiap siklus penuh (tiap jam 06.00–20.00 atau lewat Analisis ulang). Hasilnya muncul di sini setelah siklus pertama.</p>
      ) : (
        <>
          <div className="kpis wa-kpis">
            <div className="kpi-t"><small>Percakapan</small><b>{fmtNum(t?.threads ?? 0)}</b><span className="dl n">{t?.dealers ?? 0} dealer · {t?.new_numbers ?? 0} nomor baru</span></div>
            <div className="kpi-t"><small>Pesan</small><b>{fmtNum(t?.inbound ?? 0)}<em>masuk</em></b><span className="dl n">{fmtNum(t?.outbound ?? 0)} balasan sales</span></div>
            <div className="kpi-t"><small>Menunggu balasan</small><b style={t?.needs_reply ? { color: 'var(--bad)' } : undefined}>{t?.needs_reply ?? 0}</b><span className={`dl ${t?.needs_reply ? 'bad' : 'n'}`}>lebih dari {due} · {t?.waiting ?? 0} belum dibalas</span></div>
            <div className="kpi-t"><small>Waktu balas · jam kerja</small><b>{replyText(t?.reply_median_min ?? null)}</b><span className="dl n">median · p90 {replyText(t?.reply_p90_min ?? null)} · luar jam {replyText(t?.reply_off_median_min ?? null)}</span></div>
          </div>
          {data.overview ? (
            <div className="wa-ai"><span className="ai">Ringkasan Claude (MCP)</span><p>{data.overview}</p></div>
          ) : (
            <p className="cl-empty wa-ai-empty">
              {scoped ? 'Ringkasan Claude mencakup semua sales — di halaman sales ini hanya angkanya.' : <>Belum dibaca Claude pada siklus ini — angka di atas hanya hitungan dari chat. Ringkasan dan satu baris per percakapan ditulis Claude lewat MCP: tekan <b>Analisis ulang</b>, atau minta di Claude <i>"jalankan analisis GSI Orbit"</i>.</>}
            </p>
          )}
        </>
      )}
    </div>
  )
}

const FILTERS: [WAFilter, string][] = [['all', 'Semua'], ['reply', 'Perlu dibalas'], ['waiting', 'Belum dibalas']]

/** Conversations as a data table: what each one asks, how long it waits, the AI line, and the way to the chat. */
export function WAThreadsCard({ data }: { data: WAAnalysis | undefined }) {
  const nav = useNavigate()
  const [filter, setFilter] = useState<WAFilter>('all')
  const all = useMemo(() => data?.items ?? [], [data])
  const rows = useMemo(() => waFilter(all, filter), [all, filter])
  const t = useDataTable<WAInsight, WAColumn>({ rows, text: waText, compare: WA_COMPARE, tie: waTie, initial: { column: 'menunggu', dir: 'desc' }, firstDir: waFirstDir })
  const counts = (f: WAFilter) => waFilter(all, f).length
  return (
    <div className="card">
      <CardH title="Percakapan" ai="Claude · MCP" meta="lama menunggu dan ringkasan Claude per percakapan" />
      <div className="chips wa-chips">
        {FILTERS.map(([k, l]) => {
          const n = counts(k)
          if (k !== 'all' && k !== 'reply' && n === 0) return null
          return <button key={k} className={`chip ${filter === k ? 'is-active' : ''}`} onClick={() => setFilter(k)}>{l}<span className="n">{n}</span></button>
        })}
      </div>
      <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari dealer, sales, barang, pesan…" label="Cari percakapan" meta={`${fmtNum(t.matches.length)} percakapan`} />
      {all.length === 0 ? (
        <p className="sg-hint">Belum ada chat dealer atau nomor baru dalam {data?.window_days ?? 7} hari terakhir.</p>
      ) : t.matches.length === 0 ? (
        <p className="sg-hint">Tidak ada percakapan yang cocok.</p>
      ) : (
        <div className="odl-wrap">
          <table className="odl wa-table">
            <thead><tr>
              <SortTh t={t} c="percakapan">Percakapan</SortTh><SortTh t={t} c="sales">Sales</SortTh>
              <SortTh t={t} c="menunggu" right>Menunggu</SortTh><SortTh t={t} c="balas" right>Waktu balas</SortTh><SortTh t={t} c="pesan">Ringkasan Claude · pesan terakhir</SortTh><th />
            </tr></thead>
            <tbody>
              {t.shown.map((r) => (
                <tr key={r.thread_id} className={r.needs_reply ? 'wa-due' : undefined}>
                  <td>
                    {r.dealer_id ? <button className="ev" onClick={() => nav('/dealer/' + r.dealer_id)}>{who(r)}</button> : <b>{r.title}</b>}
                    <div className="wa-sub">{r.kind === 'new' ? <Pill tone="indigo">nomor baru</Pill> : r.title.split(' · ')[0]}</div>
                  </td>
                  <td className="nowrap">{r.sales || <span style={{ color: 'var(--text-3)' }}>—</span>}</td>
                  <td className="r num nowrap">
                    {r.waiting_since ? <span style={{ color: r.needs_reply ? 'var(--bad)' : 'var(--warn)', fontWeight: 600 }}>{waitLabel(r.wait_min)}</span> : r.answered ? <Pill tone="good" icon="check">dibalas</Pill> : <span style={{ color: 'var(--text-3)' }}>—</span>}
                  </td>
                  <td className="r num nowrap">{replyText(r.reply_median_min)}</td>
                  <td className="wa-msg">
                    {r.summary ? <><span className="ai" title="ditulis Claude lewat MCP" />{r.summary}</> : r.last_text ? <q>{r.last_text}</q> : <span style={{ color: 'var(--text-3)' }}>hanya pesan sales</span>}
                  </td>
                  <td className="r"><button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => nav('/chat/' + r.thread_id)}>Buka chat</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <TablePager t={t} noun="percakapan" />
    </div>
  )
}

/** Orchestrator: what each cycle read from WhatsApp. */
export function WAHistoryCard({ rows }: { rows: WAHistoryRow[] }) {
  return (
    <div className="card">
      <CardH title="Riwayat baca WhatsApp" meta="per siklus · seluruh tim" />
      {rows.length === 0 ? <p className="sg-hint">Belum ada siklus yang membaca WhatsApp.</p> : (
        <div className="odl-wrap">
          <table className="odl wa-hist">
            <thead><tr><th>Waktu</th><th className="r">No.</th><th className="r">Chat</th><th className="r">Menunggu</th><th className="r">Balas</th><th>Dibaca</th></tr></thead>
            <tbody>{rows.slice(0, 12).map((h) => (
              <tr key={`${h.cycle_number}-${h.at}`}>
                <td className="num nowrap">{shortDate(h.at)} · {hhmm(h.at)}</td>
                <td className="r num">#{fmtNum(h.cycle_number ?? 0)}</td>
                <td className="r num">{h.threads}</td>
                <td className="r num">{h.waiting}</td>
                <td className="r num">{replyText(h.reply_median_min)}</td>
                <td>{h.source === 'ai' ? <Pill tone="indigo">Claude</Pill> : <Pill tone="neutral">angka</Pill>}{h.status === 'failed' && <span style={{ color: 'var(--bad)', fontSize: 12, marginLeft: 6 }}>siklus gagal</span>}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      )}
    </div>
  )
}
