import { useState } from 'react'
import { trendChange } from './trend'

// One dashboard card's history: bars (up and down) with the line through their tops, on one axis from zero.
// Hover or tap a bar for its value; days still to come stay empty.

const W = 300
const H = 64
const PAD_T = 6

export function TrendChart({ values, labels, heads, fmt, tone, unit, better }: { values: (number | null)[]; labels: string[]; heads: string[]; fmt: (v: number) => string; tone?: string; unit: string; better: 'up' | 'down' | 'none' }) {
  const [hover, setHover] = useState<number | null>(null)
  const n = values.length
  const max = Math.max(1, ...values.map((v) => v ?? 0))
  const step = W / Math.max(1, n)
  const gap = Math.min(2, step * 0.25) // surface gap between bars
  const bw = Math.max(1, step - gap)
  const y = (v: number) => H - (v / max) * (H - PAD_T)
  const pts = values.map((v, i) => (v == null ? null : ([i * step + step / 2, y(v)] as const)))
  const path = pts.reduce((d, p, i) => (p ? d + `${d && pts[i - 1] ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}` : d), '')
  const color = `var(--${tone === 'bad' ? 'bad' : tone === 'warn' ? 'warn' : 'accent'})`
  const last = values.reduce<number>((k, v, i) => (v != null ? i : k), -1)
  const shown = hover ?? last
  const ch = trendChange(values, shown)
  const good = !ch || ch.dir === 'flat' || better === 'none' ? '' : ch.dir === better ? 'good' : 'bad'
  const r = Math.min(4, bw / 2)
  return (
    <div className="db-trend" onMouseLeave={() => setHover(null)}>
      <div className="db-trend-head" aria-live="polite">
        {shown >= 0 && values[shown] != null ? (
          <><b>{heads[shown]}</b> <span className="num">{fmt(values[shown]!)}</span>{ch && <span className={`db-trend-ch ${good}`}>{ch.dir === 'up' ? '▲' : ch.dir === 'down' ? '▼' : '■'} {ch.text}</span>}</>
        ) : <span>Belum ada data</span>}
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={`${unit}: ${labels.map((l, i) => `${l} ${values[i] == null ? '—' : fmt(values[i]!)}`).join(', ')}`}>
        <line x1="0" x2={W} y1={H - 0.5} y2={H - 0.5} className="db-trend-base" />
        {values.map((v, i) => {
          if (v == null) return null
          const top = y(v)
          const h = Math.max(v > 0 ? 1.5 : 0, H - top)
          const x = i * step + gap / 2
          const rr = Math.min(r, h)
          // rounded data end at the top, square on the baseline
          const d = `M${x},${H}V${H - h + rr}Q${x},${H - h} ${x + rr},${H - h}H${x + bw - rr}Q${x + bw},${H - h} ${x + bw},${H - h + rr}V${H}Z`
          return <path key={i} d={d} fill={color} opacity={i === shown ? 0.55 : 0.22} />
        })}
        <path d={path} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinejoin="round" strokeLinecap="round" />
        {values.map((_, i) => (
          <rect key={i} x={i * step} y="0" width={step} height={H} fill="transparent"
            onMouseEnter={() => setHover(i)} onClick={(e) => { e.preventDefault(); setHover(i) }} />
        ))}
      </svg>
      <div className="db-trend-axis"><span>{labels[0]}</span><span>{labels[Math.floor((n - 1) / 2)]}</span><span>{labels[n - 1]}</span></div>
    </div>
  )
}
