import type { Tone } from '../api/types'

// Same formatter as the mockup: billions -> "Rp 2,4 M" (2 decimals when fractional), millions -> "Rp 412 jt".
export const fmtRp = (v: number): string =>
  v >= 1e9
    ? 'Rp ' + (v / 1e9).toFixed(v % 1e9 ? 2 : 1).replace(/\.?0+$/, '').replace('.', ',') + ' M'
    : 'Rp ' + Math.round(v / 1e6) + ' jt'

// Aggregates with one decimal: "Rp 11,9 M".
export const fmtRp1 = (v: number): string =>
  v === 0 ? 'Rp 0' : v >= 1e9 ? 'Rp ' + (v / 1e9).toFixed(1).replace('.', ',') + ' M' : fmtRp(v)

export const hb = (h: number): Tone => (h >= 70 ? 'good' : h >= 50 ? 'warn' : 'bad')
export const hcol = (h: number): string => `var(--${hb(h)})`
export const fmtNum = (n: number): string => n.toLocaleString('id-ID')

export const nowHHMM = (): string => {
  const d = new Date()
  return String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0')
}
