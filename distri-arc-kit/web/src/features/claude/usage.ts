// MCP Claude → Pemakaian AI (pure, tested): formatting and the history table of AI analyses.

export interface UsageTotals { calls: number; tokens_in: number; tokens_out: number; cost_idr: number }
export interface UsageModel extends UsageTotals { provider: string; model: string; last_at: string }
export interface AIRun {
  kind: 'cycle' | 'schedule'
  id: string
  title: string
  trigger: 'schedule' | 'manual' | 'mcp'
  by: string
  via: string
  status: string
  started_at: string
  duration_ms: number | null
  model: string
  calls: number
  tokens_in: number
  tokens_out: number
  cost_idr: number
}
export interface AIUsage {
  orchestrator: { engine: 'claude' | 'template'; mode: string; provider: string; model: string; fallback: string; from_hour: number; to_hour: number; next_run_at: string }
  analyst: { engine: 'claude' | 'template'; model: string; daily_budget_idr: number; spent_today_idr: number; runs_today: number }
  cost: { today: UsageTotals; d7: UsageTotals; d30: UsageTotals; by_model: UsageModel[]; daily: { day: string; calls: number; cost_idr: number }[] }
  prices: Record<string, { in_usd_per_mtok: number; out_usd_per_mtok: number }>
  idr_per_usd: number
  runs: AIRun[]
}

/** Rupiah of AI costs, which are small: "Rp0", "Rp1.250", "Rp1,2 jt". */
export function rpAI(v: number): string {
  if (v >= 1e6) return 'Rp' + (v / 1e6).toFixed(1).replace('.', ',') + ' jt'
  return 'Rp' + Math.round(v).toLocaleString('id-ID')
}

/** Tokens: "850", "12,4 rb", "1,2 jt". */
export function tok(n: number): string {
  if (n >= 1e6) return (n / 1e6).toFixed(1).replace('.', ',') + ' jt'
  if (n >= 1e3) return (n / 1e3).toFixed(1).replace('.', ',') + ' rb'
  return String(n)
}

/** "1,2 dtk", "3 mnt 5 dtk". */
export function dur(ms: number | null): string {
  if (ms == null) return '—'
  if (ms < 60000) return (ms / 1000).toFixed(1).replace('.', ',') + ' dtk'
  const s = Math.round(ms / 1000)
  return `${Math.floor(s / 60)} mnt ${s % 60} dtk`
}

export const TRIGGER: Record<string, string> = { schedule: 'terjadwal', manual: 'manual', mcp: 'lewat MCP' }
export const MODE: Record<string, string> = { api: 'API AI', mcp: 'MCP', both: 'API AI + MCP' }

/** "Tiap jam 06.00–20.00 WIB". */
export const hourly = (from: number, to: number) => `Tiap jam ${String(from).padStart(2, '0')}.00–${String(to).padStart(2, '0')}.00 WIB`

/** A model name for people: "claude-sonnet-5-5" → "Claude Sonnet 5.5"; "template" stays explained. */
export function modelLabel(m: string): string {
  if (!m) return '—'
  if (m === 'template') return 'Template (tanpa model)'
  return m.split(', ').map((x) => {
    const c = /^claude-([a-z]+)-(\d+)-(\d+)$/.exec(x)
    return c ? `Claude ${c[1][0].toUpperCase()}${c[1].slice(1)} ${c[2]}.${c[3]}` : x
  }).join(', ')
}

export type RunColumn = 'started_at' | 'title' | 'model' | 'tokens' | 'cost_idr' | 'duration_ms'
export const runText = (r: AIRun) => `${r.title} ${r.model} ${modelLabel(r.model)} ${TRIGGER[r.trigger] ?? r.trigger} ${r.by} ${r.status}`
export const RUN_COMPARE: Record<RunColumn, (a: AIRun, b: AIRun) => number> = {
  started_at: (a, b) => a.started_at.localeCompare(b.started_at),
  title: (a, b) => a.title.localeCompare(b.title, 'id'),
  model: (a, b) => a.model.localeCompare(b.model),
  tokens: (a, b) => a.tokens_in + a.tokens_out - (b.tokens_in + b.tokens_out),
  cost_idr: (a, b) => a.cost_idr - b.cost_idr,
  duration_ms: (a, b) => (a.duration_ms ?? -1) - (b.duration_ms ?? -1),
}
export const runFirstDir = (c: RunColumn): 'asc' | 'desc' => (c === 'title' || c === 'model' ? 'asc' : 'desc')
