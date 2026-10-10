// MCP Claude → Pemakaian AI (pure, tested): formatting and the history table of AI analyses.

export interface UsageTotals { calls: number; tokens_in: number; tokens_out: number; cost_idr: number; template_steps?: number }
export interface UsageModel extends UsageTotals { provider: string; model: string; last_at: string }
export interface AIRun {
  kind: 'cycle' | 'schedule' | 'mcp'
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
  /** steps answered without a model (template text from the agent's rules, no cost) */
  template_steps?: number
  agents?: number
  failed_agents?: number
  /** proposals Claude sent through MCP for this cycle */
  mcp_proposals?: number
}
export interface AIUsage {
  /** policy llm.routing.require_ai: every analysis from a model; no answer = failed (no template) */
  require_ai?: boolean
  mcp_wait_sec?: number
  orchestrator: { engine: 'claude' | 'template' | 'mcp'; mode: string; provider: string; model: string; fallback: string; from_hour: number; to_hour: number; next_run_at: string }
  /** Claude's own connections through MCP (claude.ai, Desktop, Code) — paid by the Claude subscription */
  mcp: { connections: { name: string; kind: string; user: string; last_seen_at: string | null; calls_today: number }[]; calls_today: number; sessions_30d: number; last_at: string | null }
  analyst: { engine: 'claude' | 'template' | 'none'; model: string; daily_budget_idr: number; spent_today_idr: number; runs_today: number }
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
  if (m === 'template' || m === 'fake') return 'Template (tanpa AI)'
  if (m === 'claude.ai') return 'Claude (akun claude.ai)'
  if (m === 'none') return 'Tanpa AI — gagal'
  return m.split(', ').map((x) => {
    if (x === 'fake' || x === 'template') return 'Template (tanpa AI)'
    if (x === 'claude.ai') return 'Claude (akun claude.ai)'
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

// ---------- Riwayat MCP (every tool call Claude made) ----------
export interface MCPCallRow { id: string; client_name: string | null; client_kind?: string | null; user_name?: string | null; tool: string; args: Record<string, unknown> | null; result_summary: string | null; status: string; duration_ms: number | null; created_at: string }
export type CallColumn = 'created_at' | 'client' | 'tool' | 'status' | 'duration_ms'
export const KIND: Record<string, string> = { oauth: 'Claude (login)', bearer: 'Token manual', schedule: 'Analisis terjadwal' }
/** "dealer_id=mitra, limit=10" — the arguments of a call, short. */
export function argsText(a: Record<string, unknown> | null): string {
  if (!a) return ''
  return Object.entries(a).map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`).join(', ')
}
export const callText = (c: MCPCallRow) => `${c.client_name ?? ''} ${c.user_name ?? ''} ${c.tool} ${argsText(c.args)} ${c.result_summary ?? ''} ${c.status}`
export const CALL_COMPARE: Record<CallColumn, (a: MCPCallRow, b: MCPCallRow) => number> = {
  created_at: (a, b) => a.created_at.localeCompare(b.created_at),
  client: (a, b) => (a.client_name ?? '').localeCompare(b.client_name ?? '', 'id'),
  tool: (a, b) => a.tool.localeCompare(b.tool),
  status: (a, b) => a.status.localeCompare(b.status),
  duration_ms: (a, b) => (a.duration_ms ?? -1) - (b.duration_ms ?? -1),
}
export const callFirstDir = (c: CallColumn): 'asc' | 'desc' => (c === 'created_at' || c === 'duration_ms' ? 'desc' : 'asc')

/** "10 menit", "25 detik" — how long a cycle waits for Claude's analysis through MCP. */
export const waitText = (sec = 600) => (sec >= 60 ? `${Math.round(sec / 60)} menit` : `${sec} detik`)

/** Agents of a cycle a model analysed: none for a template cycle (before require_ai), else those not failed. */
export const aiAgents = (r: Pick<AIRun, 'model' | 'agents' | 'failed_agents'>) =>
  !r.agents || r.model === 'template' || r.model === 'none' ? 0 : r.agents - (r.failed_agents ?? 0)
