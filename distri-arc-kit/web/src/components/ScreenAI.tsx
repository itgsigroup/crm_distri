import type { ScreenAnalysis } from '../api/types'
import { hhmm, shortDate } from '../lib/format'
import { Icon } from './Icon'
import { Prov } from './ui'

// Analisa Stok / Analisa Kas (ADR 0030): what Claude wrote for a page through MCP — the overview at the top of the
// card, one note per row. The numbers stay GSI Orbit's; nothing here is GSI Orbit's own judgement.

/** The AI overview of a page, with its provenance; or how to get one. */
export function ScreenBrief({ a, what }: { a: ScreenAnalysis | null | undefined; what: string }) {
  if (!a) {
    return (
      <p className="scr-ai empty"><Icon name="spark" />Belum dianalisis AI — tekan <b>Analisis ulang</b>: Claude membaca {what} lewat MCP dan menulis ringkasan serta catatan per baris di sini.</p>
    )
  }
  return (
    <div className="scr-ai">
      <p><span className="ai" style={{ fontSize: 0 }} />{a.overview}</p>
      <Prov icon="spark">ditulis AI{a.cycle_number ? ` · siklus #${a.cycle_number}` : ''} · {shortDate(a.generated_at)} {hhmm(a.generated_at)}</Prov>
    </div>
  )
}

/** The AI note of one row ("" when Claude did not write one for it). */
export function ScreenNote({ a, k }: { a: ScreenAnalysis | null | undefined; k: string }) {
  const l = a?.lines[k]
  if (!l) return null
  return <span className="scr-note" title={`ditulis AI · confidence ${l.confidence}%`}><span className="ai" style={{ fontSize: 0 }} />{l.note}</span>
}
