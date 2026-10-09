import type { RelasiEdge, RelasiNode } from '../../api/types'

// Tooltip content shared by the relasi views.

export function esc(s: string) {
  return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)
}

export function tipHtml(d: RelasiNode, edges: RelasiEdge[], nameOf: (id: string) => string, periodLabel: string, monthLabels: string[]): string {
  if (d.type === 'sales') {
    const es = edges.filter((e) => e.sales === d.id && e.w > 0).sort((a, b) => b.w - a.w)
    return `<b>${esc(d.name)} · ${esc(d.sub)}</b><div class="r"><span>Nomor</span><span>${esc(d.number ?? '')}</span></div><div class="r"><span>Kontak aktif</span><span>${es.length}</span></div><div class="r"><span>Pesan ${periodLabel}</span><span>${d.total}</span></div>${es[0] ? `<div class="r"><span>Terkuat</span><span>${esc(nameOf(es[0].dealer))}</span></div>` : ''}`
  }
  const es = edges.filter((e) => e.dealer === d.id && e.w > 0).sort((a, b) => b.w - a.w)
  const series = edges.filter((e) => e.dealer === d.id).reduce((acc, e) => acc.map((x, i) => x + (e.monthly[i] ?? 0)), [0, 0, 0, 0, 0, 0])
  const ml = monthLabels
  return `<b>${esc(d.name)}</b><div class="r"><span>${esc(d.sub)}</span></div>${d.score != null ? `<div class="r"><span>Skor dealer</span><span>${d.score}</span></div>` : ''}<div class="r"><span>Pesan ${periodLabel}</span><span>${d.total}</span></div>${es.map((e) => `<div class="r"><span>↔ ${esc(nameOf(e.sales))}</span><span>${e.w}</span></div>`).join('')}<div class="r" style="margin-top:4px;opacity:.75"><span>${ml[0]}→${ml[ml.length - 1]}</span><span>${series.join(' · ')}</span></div><div class="r" style="margin-top:4px;opacity:.7"><span>Klik dua kali untuk buka dealer</span></div>`
}
