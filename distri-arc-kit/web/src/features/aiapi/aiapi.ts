// Pengaturan → API & model AI (pure, tested): the second option's state and what the API tokens cost.

import type { UsageTotals } from '../claude/usage'

export type ProviderID = 'claude' | 'openai'
export type Fallback = 'off' | ProviderID

export interface AIProvider { id: ProviderID; label: string; key_source: '' | 'ui' | 'env'; key_hint: string; model: string; models: { id: string; label: string }[] }
export interface UsageDay { day: string; provider: string; calls: number; tokens_in: number; tokens_out: number; cost_idr: number }
export interface UsageModel extends UsageTotals { provider: string; model: string; last_at: string }
export interface FallbackRun { id: string; schedule: string; status: string; engine: string; model: string; tokens_in: number; tokens_out: number; cost_idr: number; error: string | null; started_at: string; finished_at: string | null }
export interface AIAPIInfo {
  fallback: Fallback
  daily_budget_idr: number
  spent_today_idr: number
  /** the second option is on, its key is there and today's budget is not spent */
  ready: boolean
  providers: AIProvider[]
  mcp_wait_sec: number
  fallback_window_sec: number
  can_configure: boolean
  usage: { totals: Record<'today' | 'd7' | 'd30' | 'month', UsageTotals>; by_model: UsageModel[]; daily: UsageDay[] }
  runs: FallbackRun[]
  prices: Record<string, { in_usd_per_mtok: number; out_usd_per_mtok: number }>
  idr_per_usd: number
}

/** llm_calls.provider → the page's provider (anthropic is the Claude API). */
export function providerOf(p: string): ProviderID | 'other' {
  if (p === 'anthropic' || p === 'claude') return 'claude'
  if (p === 'openai') return 'openai'
  return 'other'
}

export const PROVIDER_LABEL: Record<ProviderID | 'other', string> = { claude: 'Claude API', openai: 'ChatGPT API', other: 'Lainnya' }

/** Cost and tokens per provider over the given model rows (30 days), largest cost first. */
export function byProvider(rows: UsageModel[]): { provider: ProviderID | 'other'; calls: number; tokens_in: number; tokens_out: number; cost_idr: number }[] {
  const m = new Map<ProviderID | 'other', { provider: ProviderID | 'other'; calls: number; tokens_in: number; tokens_out: number; cost_idr: number }>()
  for (const r of rows) {
    const p = providerOf(r.provider)
    const x = m.get(p) ?? { provider: p, calls: 0, tokens_in: 0, tokens_out: 0, cost_idr: 0 }
    x.calls += r.calls
    x.tokens_in += r.tokens_in
    x.tokens_out += r.tokens_out
    x.cost_idr += r.cost_idr
    m.set(p, x)
  }
  return [...m.values()].sort((a, b) => b.cost_idr - a.cost_idr)
}

/** One bar per day of the last `days` days ending at `today` (YYYY-MM-DD), the cost split by provider; empty days
 * included so the chart keeps its time axis. */
export function dailyBars(rows: UsageDay[], today: string, days: number): { day: string; claude: number; openai: number; other: number; total: number }[] {
  const out: { day: string; claude: number; openai: number; other: number; total: number }[] = []
  const end = new Date(today + 'T00:00:00Z')
  const idx = new Map<string, number>()
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(end.getTime() - i * 86400000).toISOString().slice(0, 10)
    idx.set(d, out.length)
    out.push({ day: d, claude: 0, openai: 0, other: 0, total: 0 })
  }
  for (const r of rows) {
    const i = idx.get(r.day.slice(0, 10))
    if (i == null) continue
    const b = out[i]
    b[providerOf(r.provider)] += r.cost_idr
    b.total += r.cost_idr
  }
  return out
}

/** "10 menit", "45 detik". */
export function waitLabel(sec: number): string {
  if (sec < 60) return `${sec} detik`
  return `${Math.round(sec / 60)} menit`
}

/** What the second option does, in one sentence for the page header. */
export function fallbackText(info: Pick<AIAPIInfo, 'fallback' | 'ready' | 'providers' | 'mcp_wait_sec' | 'daily_budget_idr' | 'spent_today_idr'>): string {
  if (info.fallback === 'off') return 'Mati — analisis hanya dari Claude lewat MCP; yang tidak dijawab dicatat Gagal.'
  const p = info.providers.find((x) => x.id === info.fallback)
  const name = `${p?.label ?? info.fallback} (${p?.model ?? ''})`
  if (!p?.key_source) return `${name} dipilih, tetapi kuncinya belum diisi — belum aktif.`
  if (!info.ready && info.daily_budget_idr > 0 && info.spent_today_idr >= info.daily_budget_idr) return `${name} berhenti hari ini: anggaran harian sudah terpakai.`
  return `Aktif — bila Claude lewat MCP belum menjawab dalam ${waitLabel(info.mcp_wait_sec)}, ${name} di server mengerjakan sisanya.`
}
