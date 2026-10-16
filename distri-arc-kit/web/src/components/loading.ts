import { useEffect, useRef, useState } from 'react'

// Loading popup (16 Oct): what is loading, and when the popup shows.

/** Shows a flag `delay` ms after `active` turns on (fast loads never flash the popup) and keeps it at least
 * `minShow` ms once shown (no blink when the load ends right after). */
export function useDelayedFlag(active: boolean, delay = 350, minShow = 500): boolean {
  const [shown, setShown] = useState(false)
  const since = useRef(0)
  useEffect(() => {
    if (active && !shown) {
      const t = setTimeout(() => { since.current = Date.now(); setShown(true) }, delay)
      return () => clearTimeout(t)
    }
    if (!active && shown) {
      const t = setTimeout(() => setShown(false), Math.max(0, minShow - (Date.now() - since.current)))
      return () => clearTimeout(t)
    }
  }, [active, shown, delay, minShow])
  return shown
}

const LABELS: Record<string, string> = {
  me: 'akun', dealers: 'dealer', dealer: 'dealer', orbit: 'orbit', segmen: 'segmen', chat: 'chat', stock: 'stok', kpi: 'KPI',
  agenda: 'agenda', brief: 'ringkasan', proposals: 'keputusan', queue: 'keputusan', cycles: 'siklus Orchestrator', cycle: 'siklus Orchestrator',
  agents: 'agen', trend: 'tren', credit: 'kredit', ar: 'piutang', sales: 'sales', wa: 'WhatsApp', policies: 'kebijakan',
  'reply-rules': 'AI Trigger', users: 'pengguna', roles: 'peran', branches: 'cabang', health: 'status sistem', connections: 'koneksi',
}

/** "Memuat dealer, chat…" from the first key part of each loading query (unknown keys count as "data"). */
export function loadingText(keys: readonly unknown[][]): string {
  const names: string[] = []
  for (const k of keys) {
    const n = LABELS[String(k[0])] ?? 'data'
    if (!names.includes(n)) names.push(n)
  }
  if (names.length === 0) return 'Memuat data…'
  const shown = names.slice(0, 3).join(', ')
  return `Memuat ${shown}${names.length > 3 ? ` +${names.length - 3}` : ''}…`
}
