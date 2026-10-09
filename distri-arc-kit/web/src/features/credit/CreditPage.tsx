import { useMemo } from 'react'
import { useNavigate } from 'react-router'
import { Icon } from '../../components/Icon'
import { ActBtn } from '../../components/actions'
import { fmtRp } from '../../lib/format'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useCreditDealers, useCreditExposure, useCreditForecast, useCreditOverview, useOrbit } from '../../app/queries'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { EXPOSURE_COMPARE, exposureFirstDir, exposureLines, exposureText, exposureTie, type ExposureColumn, type ExposureLine } from './exposure'

const CREDIT_KINDS = new Set(['collect', 'installment', 'credit_release', 'credit_limit'])

// Kredit · kas (mockup screen-ar).
export function CreditPage() {
  const nav = useNavigate()
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  const { data: ov } = useCreditOverview()
  const { data: ar = [] } = useCreditDealers()
  const { data: ex = [] } = useCreditExposure()
  const { data: fc = [] } = useCreditForecast()
  const { data: board = [] } = useOrbit()
  const next = new Map(board.map((d) => [d.id, d.next]))
  const lines = useMemo(() => exposureLines(ex, board), [ex, board])

  // the four largest receivables, ordered by what is expected to come in (mockup)
  const byOpen = [...fc].sort((a, b) => b.open - a.open)
  const top = byOpen.slice(0, 4).sort((a, b) => b.expected - a.expected)
  const rest = byOpen.slice(4)
  const restOpen = rest.reduce((a, f) => a + f.open, 0)
  const restExp = rest.reduce((a, f) => a + f.expected, 0)
  const restPay = rest.length ? Math.round(rest.reduce((a, f) => a + f.pay_days * f.open, 0) / Math.max(1, restOpen)) : 0

  return (
    <>
      <div className="kpis">
        <div className="kpi-t"><small>DSO (order → bayar) · order → bayar</small><b className="num">{ov?.dso_days ?? '—'}<em>hari</em></b><span className={`dl ${ov && ov.dso_days <= ov.terms_avg + 5 ? 'good' : 'warn'}`}>termin rata-rata {ov?.terms_avg ?? '—'}</span></div>
        <div className="kpi-t"><small>Piutang dealer</small><b className="num">{fmtRp(ov?.receivable ?? 0)}</b><span className="dl n">{ov?.open_invoices ?? 0} invoice · {ov?.open_dealers ?? 0} dealer</span></div>
        <div className="kpi-t"><small>Lewat tempo</small><b className="num">{fmtRp(ov?.overdue ?? 0)}</b><span className="dl bad">{ov?.overdue_dealers ?? 0} dealer · {ov?.overdue_over_30 ?? 0} di atas 30 hari</span></div>
        <div className="kpi-t"><small>Kas masuk 30 hari (prediksi)</small><b className="num">{fmtRp(ov?.forecast_30 ?? 0)}</b><span className="dl n">tertimbang pola bayar</span></div>
      </div>
      <div className="grid-2">
        <div className="card">
          <div className="card-h">
            <h2>Sisa limit tiap dealer</h2><span className="ai" style={{ marginLeft: 6 }}>AI Kredit</span><span className="meta">Ruang kredit tersisa · pola bayar</span>
            <button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 8 }} disabled={orch.running} onClick={() => reanalyze('screen:credit')}><Icon name="refresh" />Analisis ulang</button>
          </div>
          <ul className="l2c">
            {ar.slice(0, 5).map((x) => {
              const over = x.overdue > 0
              const k = x.late_days > 14 ? 'bad' : x.late_days > 0 ? 'warn' : 'good'
              const n = next.get(x.dealer_id)
              const act = n && n.status === 'proposed' && CREDIT_KINDS.has(n.kind) ? n : null
              return (
                <li key={x.dealer_id}>
                  <div><b><button className="ev" onClick={() => nav('/dealer/' + x.dealer_id)}>{x.name}</button></b><span className="s">pola {x.pay_days ? x.pay_days + ' hari' : 'cash'} · {x.on_time}% tepat · {x.owner}</span></div>
                  <div>
                    <div className="seg5"><i className={over ? `cur ${k}` : 'done'} /><i className={over ? '' : 'done'} /><i className={over ? '' : 'done'} /><i /><i /></div>
                    <div className="stg"><span>{over ? 'lewat jatuh tempo' : 'belum jatuh tempo'}</span><em className="num" style={{ color: `var(--${k})` }}>{over ? x.late_days + ' hr' : 'OK'}</em></div>
                  </div>
                  <div className="note">Terbuka {fmtRp(x.open)}{over && <> · <b style={{ color: 'var(--bad)' }}>lewat {fmtRp(x.overdue)}</b></>}</div>
                  <div>{act ? <ActBtn small next={act} /> : <span className="pill good"><Icon name="check" />Pengingat H-3 otomatis</span>}</div>
                </li>
              )
            })}
          </ul>
        </div>
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Prediksi kas masuk 30 hari</h2></div>
            <ul className="cin">
              {top.map((f) => (
                <li key={f.dealer_id}>
                  <div><b>{f.name}</b><span>{f.invoices > 1 ? `${f.invoices} invoice ` : ''}{fmtRp(f.open)} · pola {f.pay_days} hari{f.asked_tempo ? ' · minta tempo' : f.late_count ? ` · ${f.late_count} lewat` : ''}</span></div>
                  <div className="p"><i style={{ width: `${Math.round(f.probability * 100)}%` }} /></div>
                  <em className="num">{fmtRp(f.expected)}</em>
                </li>
              ))}
              {rest.length > 0 && (
                <li>
                  <div><b>{rest.length} dealer lain</b><span>{fmtRp(restOpen)} · rata-rata pola {restPay} hari</span></div>
                  <div className="p"><i style={{ width: `${Math.round((restExp / Math.max(1, restOpen)) * 100)}%` }} /></div>
                  <em className="num">{fmtRp(restExp)}</em>
                </li>
              )}
            </ul>
          </div>
        </div>
      </div>
      <ExposureTable rows={lines} onOpen={(id) => nav('/dealer/' + id)} />
    </>
  )
}

const tone = (pct: number) => (pct > 100 ? 'bad' : pct > 80 ? 'warn' : 'good')

/** "Exposure vs limit" as a data table: search, sort every column both ways, pages of 10/25/50. */
function ExposureTable({ rows, onOpen }: { rows: ExposureLine[]; onOpen: (id: string) => void }) {
  const t = useDataTable<ExposureLine, ExposureColumn>({ rows, text: exposureText, compare: EXPOSURE_COMPARE, tie: exposureTie, initial: { column: 'pct', dir: 'desc' }, firstDir: exposureFirstDir })
  const over = t.matches.filter((x) => x.pct > 100).length
  return (
    <div className="card exposure-card">
      <div className="card-h"><h2>Exposure vs limit</h2><span className="meta">Piutang terbuka dibanding limit kredit tiap dealer</span></div>
      <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari dealer, sales, cabang…" label="Cari exposure dealer" meta={<>{t.matches.length.toLocaleString('id-ID')} dealer{over ? <> · <b style={{ color: 'var(--bad)' }}>{over} over limit</b></> : null}</>} />
      {t.matches.length === 0 ? (
        <p className="sg-hint">{rows.length ? 'Tidak ada dealer yang cocok dengan pencarian ini.' : 'Belum ada dealer dengan limit kredit.'}</p>
      ) : (
        <div className="odl-wrap">
          <table className="odl exposure-table">
            <thead><tr>
              <SortTh t={t} c="dealer">Dealer</SortTh>
              <SortTh t={t} c="exposure" right>Exposure</SortTh>
              <SortTh t={t} c="limit" right>Limit</SortTh>
              <SortTh t={t} c="room" right>Sisa limit</SortTh>
              <SortTh t={t} c="pct">Pemakaian limit</SortTh>
            </tr></thead>
            <tbody>
              {t.shown.map((x) => (
                <tr key={x.dealer_id}>
                  <td className="odl-name"><button className="ev" onClick={() => onOpen(x.dealer_id)}>{x.short_name}</button><small>{[x.owner, x.branch].filter(Boolean).join(' · ') || x.name}</small></td>
                  <td className="odl-rp">{fmtRp(x.exposure)}</td>
                  <td className="odl-rp">{fmtRp(x.limit)}</td>
                  <td className="odl-rp" style={{ color: x.room < 0 ? 'var(--bad)' : undefined }}>{x.room < 0 ? '−' + fmtRp(-x.room) : fmtRp(x.room)}</td>
                  <td className="ex-use">
                    <div className="ex-bar"><i style={{ width: `${Math.max(0, Math.min(100, x.pct))}%`, background: `var(--${tone(x.pct)})` }} /></div>
                    <em className="num" style={{ color: `var(--${tone(x.pct)})` }}>{x.pct}%</em>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <TablePager t={t} noun="dealer" />
      <p style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 12, lineHeight: 1.5 }}>Limit naik bila ≥ 12 bulan tepat waktu + siklus order naik; turun bila 2 invoice berturut lewat &gt; 14 hari.</p>
    </div>
  )
}
