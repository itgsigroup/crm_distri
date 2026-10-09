// Segmen chart scales — port of renderKuad() in the approved mockup.
// X: orders per month, linear 0–3.5. Y: Rp per order, log 4 jt–200 jt.
export const W = 760
export const H = 560
export const L = 70
export const R = 22
export const T = 30
export const B = 64
export const XMAX = 3.5
export const YMIN = 4e6
export const YMAX = 200e6

export const px = (x: number) => L + (x / XMAX) * (W - L - R)
export const py = (v: number) => T + (1 - (Math.log(v) - Math.log(YMIN)) / (Math.log(YMAX) - Math.log(YMIN))) * (H - T - B)
/** Values are clamped inside the plot so extreme dealers stay visible; points themselves are never moved. */
export const clampY = (v: number) => py(Math.max(YMIN * 1.15, Math.min(YMAX / 1.1, v)))
/** Dealers without a rhythm (Baru) sit at 0,22× on the left edge. */
export const xOf = (freq: number | null) => (freq == null ? px(0.22) : px(Math.min(XMAX - 0.1, freq)))

/** Inverse of px / py: the order frequency and Rp value at a chart coordinate. */
export const freqAt = (x: number) => ((x - L) / (W - L - R)) * XMAX
export const valueAt = (y: number) => Math.exp(Math.log(YMIN) + (1 - (y - T) / (H - T - B)) * (Math.log(YMAX) - Math.log(YMIN)))

/** X-axis ticks for the visible frequency range: the largest step that still gives at least 5 ticks. */
export function xTicks(f0: number, f1: number): number[] {
  const span = f1 - f0
  const step = [0.5, 0.25, 0.1, 0.05, 0.025].find((s) => span / s >= 5) ?? 0.025
  const out: number[] = []
  for (let k = Math.ceil(f0 / step - 1e-9); k * step <= f1 + 1e-9; k++) if (k > 0) out.push(Math.round(k * step * 1000) / 1000)
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
  v >= 1e9 ? `${(v / 1e9).toFixed(1).replace(/\.0$/, '').replace('.', ',')} M` : `${(v / 1e6).toFixed(v < 1e7 && v % 1e6 ? 1 : 0).replace('.', ',')} jt`
