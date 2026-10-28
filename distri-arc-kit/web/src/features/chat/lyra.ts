import type { LyraCard, LyraLine } from '../../api/types'

// Lyra di bawah bubble (ADR 0070): the discount maths and the message the sales sends. Stock and DPP prices come from
// GSI Orbit; the discount and quantities are the sales' own, set on the card before sending.

/** "Rp 1.643.500" — full rupiah, as a customer reads it. */
export const rupiah = (v: number) => 'Rp ' + Math.round(v).toLocaleString('id-ID')

/** A typed discount ("5", "2,5", "12.5%") as a percentage between 0 and 100 (two decimals at most). */
export function parsePct(s: string): number {
  const v = Number.parseFloat(s.replace(',', '.').replace(/[^\d.]/g, ''))
  if (!Number.isFinite(v) || v <= 0) return 0
  return Math.min(100, Math.round(v * 100) / 100)
}

/** The unit price after the discount, rounded to the rupiah. */
export const discounted = (price: number, pct: number) => Math.round((price * (100 - pct)) / 100)

/** The quantity a line starts with: what the customer asked, else 1. */
export const startQty = (l: LyraLine) => (l.asked > 0 ? l.asked : 1)

/** "Surabaya 30 · Semarang 12", or "kosong" when no city has it. */
export const stockText = (l: LyraLine) => (l.cities.length ? l.cities.map((c) => `${c.city} ${c.qty.toLocaleString('id-ID')}`).join(' · ') : 'kosong')

export interface QuoteRow { line: LyraLine; qty: number }

/** Each line's total after the discount and the grand total (lines without a price are left out). */
export function totals(rows: QuoteRow[], pct: number): { lines: number[]; total: number; before: number } {
  const lines = rows.map((r) => discounted(r.line.price, pct) * r.qty)
  const before = rows.reduce((s, r) => s + r.line.price * r.qty, 0)
  return { lines, total: lines.reduce((s, v) => s + v, 0), before }
}

/** The message for the customer, in WhatsApp formatting; the sales edits it before sending. */
export function quoteText(rows: QuoteRow[], pct: number): string {
  const { lines, total } = totals(rows, pct)
  const out = ['Berikut info produk yang ditanyakan:', '']
  rows.forEach((r, i) => {
    const l = r.line
    out.push(`*${rows.length > 1 ? `${i + 1}. ` : ''}${l.product}*`)
    out.push(`Stok: ${l.cities.length ? stockText(l) : 'kosong (bisa inden)'}`)
    if (l.price > 0) {
      const unit = discounted(l.price, pct)
      out.push(pct > 0 ? `Harga: ~${rupiah(l.price)}~ diskon ${fmtPct(pct)}% → *${rupiah(unit)}*/unit` : `Harga: *${rupiah(l.price)}*/unit`)
      out.push(`${r.qty.toLocaleString('id-ID')} unit × ${rupiah(unit)} = *${rupiah(lines[i])}*`)
    } else {
      out.push('Harga: kami konfirmasi segera')
    }
    out.push('')
  })
  if (rows.length > 1 && total > 0) out.push(`*Total: ${rupiah(total)}*`)
  out.push('Harga DPP, belum termasuk PPN.')
  return out.join('\n')
}

/** "2,5" for 2.5. */
export const fmtPct = (v: number) => v.toLocaleString('id-ID', { maximumFractionDigits: 2 })

/** Whether the card has anything to send (at least one product). */
export const sendable = (c: LyraCard) => c.status === 'done' && c.items.length > 0
