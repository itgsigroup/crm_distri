import { useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import type { BoardItem } from '../api/types'
import { useFeedback } from '../components/feedback'
import { useOrch } from './orch'

/** Command bar (⌘K) parser: "analisis ulang …" → Orchestrator; a dealer name → its page; else → Tanya. */
export function parseCommand(text: string, dealers: BoardItem[]):
  | { kind: 'reanalyze'; scope: string; via: 'auto' | 'mcp' }
  | { kind: 'dealer'; id: string; name: string }
  | { kind: 'ask'; q: string } {
  const ql = text.toLowerCase()
  if (/analisis ulang|re-?analis|jalankan ulang|hitung ulang/.test(ql)) {
    const d = dealers.find((x) => ql.includes(x.name.toLowerCase().replace(/^(pt|cv|ud|toko)\s/, '').split(' ')[0]))
    const scope = d ? 'dealer:' + d.id : /semua|seluruh/.test(ql) ? 'all' : /stok/.test(ql) ? 'screen:stock' : /kredit|limit|kas/.test(ql) ? 'screen:credit' : /orbit/.test(ql) ? 'screen:orbit' : /segmen/.test(ql) ? 'screen:segmen' : 'all'
    return { kind: 'reanalyze', scope, via: /mcp/.test(ql) ? 'mcp' : 'auto' }
  }
  const d = dealers.find((x) => x.name.toLowerCase().includes(ql))
  if (d && !/\?/.test(text)) return { kind: 'dealer', id: d.id, name: d.name }
  return { kind: 'ask', q: text }
}

export function useCommand() {
  const nav = useNavigate()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const { reanalyze } = useOrch()
  return (text: string) => {
    const dealers = (qc.getQueryData<BoardItem[]>(['orbit', 'all']) ?? [])
    const c = parseCommand(text, dealers)
    if (c.kind === 'dealer') {
      nav('/dealer/' + c.id)
      toast('Membuka ' + c.name)
    } else if (c.kind === 'reanalyze') {
      reanalyze(c.scope, c.via)
    } else {
      toast('Tanya dengan sumber tersedia setelah memori & kalibrasi (Stage 10)')
    }
  }
}
