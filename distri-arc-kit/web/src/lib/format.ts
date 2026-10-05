// Formatting exactly as the mockup (reference/distri-arc-orbit-v2-mockup.html › Util).

/** fmtRp: "Rp 62 jt", "Rp 1,28 M" — identical to the mockup's fmtRp. */
export const fmtRp = (v: number): string =>
  v >= 1e9
    ? 'Rp ' + (v / 1e9).toFixed(v % 1e9 ? 2 : 1).replace(/\.?0+$/, '').replace('.', ',') + ' M'
    : 'Rp ' + Math.round(v / 1e6) + ' jt'

/** Score band: ≥ 70 good, 50–69 warn, < 50 bad. */
export const hb = (h: number): 'good' | 'warn' | 'bad' => (h >= 70 ? 'good' : h >= 50 ? 'warn' : 'bad')
export const hcol = (h: number) => `var(--${hb(h)})`

/** One decimal with a comma ("2,1"). */
export const fx1 = (f: number) => f.toFixed(1).replace('.', ',')

/** Number with Indonesian thousands separator ("1.284"). */
export const fmtNum = (n: number) => n.toLocaleString('id-ID')

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des']
const MONTHS_LONG = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember']
const DAYS = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu']
const ORDINAL = ['pertama', 'kedua', 'ketiga', 'keempat', 'kelima', 'keenam', 'ketujuh', 'kedelapan', 'kesembilan', 'kesepuluh', 'kesebelas', 'kedua belas', 'ketiga belas', 'keempat belas']

/** Calendar parts in WIB regardless of the browser time zone. */
export function wib(d: Date | string) {
  const t = typeof d === 'string' ? new Date(d) : d
  const w = new Date(t.getTime() + 7 * 3600 * 1000)
  return { y: w.getUTCFullYear(), m: w.getUTCMonth(), d: w.getUTCDate(), wd: w.getUTCDay(), h: w.getUTCHours(), mi: w.getUTCMinutes() }
}

/** "2 Okt" */
export const shortDate = (d: Date | string) => {
  const p = wib(d)
  return `${p.d} ${MONTHS[p.m]}`
}

/** "06.45" (mockup clock style) */
export const hhmm = (d: Date | string) => {
  const p = wib(d)
  return String(p.h).padStart(2, '0') + '.' + String(p.mi).padStart(2, '0')
}

/** "Senin, 5 Oktober 2026 · minggu pertama Q4" */
export function todayLine(d: Date | string) {
  const p = wib(d)
  const q = Math.floor(p.m / 3) + 1
  const qStart = Date.UTC(p.y, (q - 1) * 3, 1)
  const day = Math.floor((Date.UTC(p.y, p.m, p.d) - qStart) / 86400000)
  const week = ORDINAL[Math.min(ORDINAL.length - 1, Math.floor(day / 7))]
  return `${DAYS[p.wd]}, ${p.d} ${MONTHS_LONG[p.m]} ${p.y} · minggu ${week} Q${q}`
}

/** Greeting by WIB hour. */
export function greeting(d: Date | string) {
  const h = wib(d).h
  return h < 11 ? 'Selamat pagi' : h < 15 ? 'Selamat siang' : h < 18 ? 'Selamat sore' : 'Selamat malam'
}

/** Drop the legal prefix like the mockup boards ("PT ", "CV ", "UD ", "Toko "). */
export const shortName = (n: string) => n.replace(/^(PT|CV|UD|Toko)\s/, '')

/** Contact avatar like the mockup PIC list: first two letters without Pak/Bu/Mbak/Mas. */
export const contactIni = (n: string) => n.replace(/^(Pak|Bu|Mbak|Mas)\s+/, '').slice(0, 2).toUpperCase()
