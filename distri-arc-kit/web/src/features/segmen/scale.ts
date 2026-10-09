// Segmen chart scales — port of renderKuad() in the approved mockup.
// X: orders per month, linear 0–3.5 (log above, see Domain). Y: Rp per order, log 4 jt–200 jt (wider with data).
export const W = 760
export const H = 560
export const L = 70
export const R = 22
export const T = 30
export const B = 64
export const XMAX = 3.5
export const YMIN = 4e6
export const YMAX = 200e6

/** The visible value range. The mockup's 0–3,5× and 4–200 jt stay the default; real data beyond them widens the
 * range instead of piling dealers on the edge: y (log) simply extends, x keeps 0–3,5× linear on the first 80% of the
 * width and fits everything above 3,5× on the last 20% with a log scale. */
export interface Domain { xtop: number; ymin: number; ymax: number }
export const BASE: Domain = { xtop: XMAX, ymin: YMIN, ymax: YMAX }
export const XSPLIT = 0.8

export function domainOf(points: { freq: number | null; avg_order: number }[]): Domain {
  let f = 0
  let lo = Infinity
  let hi = 0
  for (const p of points) {
    if (p.freq != null && Number.isFinite(p.freq)) f = Math.max(f, p.freq)
    if (p.avg_order > 0) { lo = Math.min(lo, p.avg_order); hi = Math.max(hi, p.avg_order) }
  }
  return {
    xtop: f > XMAX - 0.1 ? Math.min(60, Math.max(f * 1.08, XMAX * 1.3)) : XMAX,
    ymin: lo < YMIN * 1.15 ? Math.max(1e5, lo / 1.25) : YMIN,
    ymax: hi > YMAX / 1.1 ? Math.min(1e10, hi * 1.25) : YMAX,
  }
}

const PWID = W - L - R
export const px = (x: number, d: Domain = BASE) => {
  if (d.xtop <= XMAX) return L + (x / XMAX) * PWID
  if (x <= XMAX) return L + (x / XMAX) * PWID * XSPLIT
  return L + PWID * (XSPLIT + (1 - XSPLIT) * (Math.log(Math.min(x, d.xtop) / XMAX) / Math.log(d.xtop / XMAX)))
}
export const py = (v: number, d: Domain = BASE) => T + (1 - (Math.log(v) - Math.log(d.ymin)) / (Math.log(d.ymax) - Math.log(d.ymin))) * (H - T - B)
/** Values are clamped inside the plot so extreme dealers stay visible; points themselves are never moved. */
export const clampY = (v: number, d: Domain = BASE) => py(Math.max(d.ymin * 1.15, Math.min(d.ymax / 1.1, v)), d)
/** Dealers without a rhythm (Baru) sit at 0,22× on the left edge. */
export const xOf = (freq: number | null, d: Domain = BASE) => (freq == null ? px(0.22, d) : px(d.xtop <= XMAX ? Math.min(XMAX - 0.1, freq) : Math.min(d.xtop, freq), d))

/** Inverse of px / py: the order frequency and Rp value at a chart coordinate. */
export const freqAt = (x: number, d: Domain = BASE) => {
  const t = (x - L) / PWID
  if (d.xtop <= XMAX) return t * XMAX
  if (t <= XSPLIT) return (t / XSPLIT) * XMAX
  return XMAX * Math.exp(((t - XSPLIT) / (1 - XSPLIT)) * Math.log(d.xtop / XMAX))
}
export const valueAt = (y: number, d: Domain = BASE) => Math.exp(Math.log(d.ymin) + (1 - (y - T) / (H - T - B)) * (Math.log(d.ymax) - Math.log(d.ymin)))

/** X-axis ticks for the visible frequency range: the largest step that still gives at least 5 ticks. */
export function xTicks(f0: number, f1: number): number[] {
  const lin1 = Math.min(f1, XMAX)
  const out: number[] = []
  if (f0 <= XMAX) {
    const span = lin1 - f0
    const step = [0.5, 0.25, 0.1, 0.05, 0.025].find((s) => span / s >= 5) ?? 0.025
    for (let k = Math.ceil(f0 / step - 1e-9); k * step <= lin1 + 1e-9; k++) if (k > 0) out.push(Math.round(k * step * 1000) / 1000)
  }
  // above 3,5×: the compressed log part — round numbers; the caller drops labels that would touch
  if (f1 > XMAX) for (const v of [4, 4.5, 5, 6, 7, 8, 9, 10, 12, 15, 20, 25, 30, 40, 50, 60]) if (v > XMAX && v >= f0 && v <= f1) out.push(v)
  return out
}

/** Y-axis ticks (log axis) for the visible Rp range: 1-2-5 series, finer when zoomed in. */
export function yTicks(v0: number, v1: number): number[] {
  const levels = [[1, 2, 5], [1, 1.5, 2, 3, 5, 7], [1, 1.25, 1.5, 2, 2.5, 3, 4, 5, 6, 7, 8, 9]]
  let out: number[] = []
  for (const ms of levels) {
    out = []
    for (let e = 5; e <= 9; e++) for (const m of ms) {
      const v = m * 10 ** e
      if (v >= v0 && v <= v1) out.push(v)
    }
    if (out.length >= 4) break
  }
  return out
}

/** "5 jt", "1,5 jt", "150 jt", "1,2 M" — tick labels keep one decimal where the step needs it. */
export const fmtTick = (v: number) =>
  v >= 1e9 ? `${(v / 1e9).toFixed(1).replace(/\.0$/, '').replace('.', ',')} M` : v < 1e6 ? `${Math.round(v / 1e3)} rb` : `${(v / 1e6).toFixed(v < 1e7 && v % 1e6 ? 1 : 0).replace('.', ',')} jt`
