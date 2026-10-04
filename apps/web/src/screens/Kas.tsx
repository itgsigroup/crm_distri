// Kas screen — port of the mockup's #screen-cash plus renderL2C (Won → Lunas).
import { useApi } from '../api/client'
import type { Cash, L2CRow } from '../api/types'
import { Html, Icon, Loading, Pill } from '../components/ui'
import { fmtRp, fmtRp1 } from '../lib/format'
import { useUI } from '../state/ui'

const SEGMENTS = 5 // Persiapan · Pemasangan · BAST · Invoice · Menunggu bayar; stage 6 = Lunas
const LUNAS = 6

// Probability may arrive as 0–1 or 0–100; the bar needs a percentage.
const pct = (p: number) => Math.max(0, Math.min(100, p <= 1 ? p * 100 : p))

export function KasScreen() {
  const { openSheet } = useUI()
  const { data, error } = useApi<Cash>('/api/cash')
  if (!data) return <section className="screen"><Loading error={error} /></section>

  return (
    <section className="screen" id="screen-cash">
      <div className="kpis">
        {data.kpis.map(k => (
          <div className="kpi-t" key={k.label}>
            <small>{k.label}</small>
            <b className="num">{k.value}{k.unit && <em>{k.unit}</em>}</b>
            <span className={'dl ' + k.tone}>{k.delta}</span>
          </div>
        ))}
      </div>

      <div className="card" style={{ marginBottom: 20 }}>
        <div className="card-h">
          <h2>Won → Lunas</h2>
          <span className="meta">Persiapan · Pemasangan · BAST · Invoice · Lunas — hari di stage vs benchmark GSI</span>
        </div>
        <ul className="l2c" id="l2c">
          {data.l2c.map(x => <L2CItem key={x.id} x={x} />)}
        </ul>
      </div>

      <div className="grid-2">
        <div className="card">
          <div className="card-h"><h2>Umur piutang</h2><span className="meta">{data.aging.meta}</span></div>
          <div className="aging">
            {data.aging.rows.map(r => (
              <div className="row" key={r.label}>
                <span className="lbl">{r.label}</span>
                <div className="bar"><i style={{ width: r.width + '%', background: r.color || undefined }} /></div>
                <span className="v num">{r.value}<small>{r.sub}</small></span>
              </div>
            ))}
          </div>
          <div className="hr" />
          <div className="card-h" style={{ marginBottom: 8 }}>
            <h2 style={{ fontSize: 13 }}>Tindakan penagihan</h2>
            <span className="ai" style={{ marginLeft: 6 }}>Collection agent</span>
          </div>
          <ul className="row-list">
            {data.collection.map(c => (
              <li key={c.action.id}>
                <span className="who" style={{ width: 'auto' }}><Pill p={c.badge} /></span>
                <div><div className="t">{c.t}</div><div className="s">{c.s}</div></div>
                <button className="btn ghost st" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(c.action)}>{c.action.button_label}</button>
              </li>
            ))}
          </ul>
        </div>

        <div className="card">
          <div className="card-h"><h2>Prediksi kas masuk 30 hari</h2><span className="meta">Probabilitas dari pola bayar &amp; stage</span></div>
          <ul className="cin">
            {data.forecast.items.map((it, i) => (
              <li key={it.t + i}>
                <div><b>{it.t}</b><span>{it.s}</span></div>
                <div className="p" title={Math.round(pct(it.p)) + '%'}><i style={{ width: pct(it.p) + '%' }} /></div>
                <em className="num">{fmtRp(it.v)}</em>
              </li>
            ))}
            <li>
              <div><b>Total tertimbang</b></div>
              <div />
              <em className="num">{fmtRp1(data.forecast.total)}</em>
            </li>
          </ul>
          {data.forecast.note_html && (
            <div className="html-go">
              <Html as="p" html={data.forecast.note_html} style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 12, lineHeight: 1.5 }} />
            </div>
          )}
        </div>
      </div>
    </section>
  )
}

function L2CItem({ x }: { x: L2CRow }) {
  const { openSheet } = useUI()
  const cur = x.stage
  const lunas = cur === LUNAS
  const segClass = (i: number) =>
    i + 1 < cur || lunas ? 'done' : i + 1 === cur ? 'cur' + (x.tone === 'good' ? '' : ' ' + x.tone) : ''
  const dayColor = lunas ? 'var(--good)' : x.days > x.bench ? 'var(--' + (x.tone === 'bad' ? 'bad' : 'warn') + ')' : 'var(--text)'
  const a = x.action

  let right = null
  if (lunas) right = <span className="pill good"><Icon n="i-check" />Lunas</span>
  else if (a?.status === 'executed') right = <span className="pill good"><Icon n="i-check" />Dijalankan</span>
  else if (a?.status === 'rejected') right = <span className="pill bad"><Icon n="i-x" />Ditolak</span>
  else if (a) right = <button className="btn primary" onClick={() => openSheet(a)}><Icon n={a.icon} />{a.button_label}</button>

  return (
    <li>
      <div><b>{x.account}</b><span className="s">{x.sub}</span></div>
      <div>
        <div className="seg5">
          {Array.from({ length: SEGMENTS }, (_, i) => <i key={i} className={segClass(i)} />)}
        </div>
        <div className="stg">
          <span>{lunas ? 'Lunas' : x.stage_label}</span>
          <em className="num" style={{ color: dayColor }}>
            {x.days} hr <span style={{ fontWeight: 500, color: 'var(--text-3)' }}>/ {x.bench}</span>
          </em>
        </div>
      </div>
      <div className="note">{x.note}</div>
      <div>{right}</div>
    </li>
  )
}
