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
