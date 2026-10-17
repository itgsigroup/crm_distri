// Pengaturan → API & model AI (pure, tested): the second option's state and what the API tokens cost.

import type { UsageTotals } from '../claude/usage'

export type ProviderID = 'claude' | 'openai'
export type Fallback = 'off' | ProviderID

/** in_usd / out_usd: price per million tokens; null when unknown. live: the provider lists it for the saved key. */
export interface ModelEntry { id: string; label: string; in_usd: number | null; out_usd: number | null; live: boolean }
export interface AIProvider { id: ProviderID; label: string; key_source: '' | 'ui' | 'env'; key_hint: string; model: string; models: ModelEntry[] }
export interface UsageDay { day: string; provider: string; calls: number; tokens_in: number; tokens_out: number; cost_idr: number }
export interface UsageModel extends UsageTotals { provider: string; model: string; last_at: string }
export interface FallbackRun { id: string; schedule: string; status: string; engine: string; model: string; tokens_in: number; tokens_out: number; cost_idr: number; error: string | null; started_at: string; finished_at: string | null }
/** Analisa WhatsApp otomatis (ADR 0043): the server runs it at these hours (WIB) with one API and model, no fallback. */
export interface WAAuto { enabled: boolean; hours: number[]; weekend: boolean; engine: ProviderID; model: string; next_run_at?: string | null }
export interface AIAPIInfo {
  wa_auto: WAAuto
  wa_auto_runs: FallbackRun[] | null
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
  /** costs are stored in rupiah (llm_calls.cost_idr); the page shows dollars at this rate */
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

/** A cost in dollars from rupiah: "$0", "<$0.01", "$0.42", "$12.30", "$1,250". */
export function usd(idr: number, rate: number): string {
  const v = rate > 0 ? idr / rate : 0
  if (v === 0) return '$0'
  if (v < 0.01) return '<$0.01'
  if (v >= 1000) return '$' + Math.round(v).toLocaleString('en-US')
  return '$' + v.toFixed(2)
}

/** A price per million tokens: "$4", "$0.10", "$1.25". */
export function price(v: number | null): string {
  if (v == null) return '?'
  return '$' + (Number.isInteger(v) ? String(v) : v.toFixed(2))
}

/** "Claude Opus 5.5 · $4 / $20" — a model with its input / output price per million tokens. */
export function modelOption(m: ModelEntry): string {
  const label = m.label || m.id
  return m.in_usd == null ? `${label} · harga ?` : `${label} · ${price(m.in_usd)} / ${price(m.out_usd)}`
}

/** Daily budget choices in dollars (stored in rupiah at the rate). */
export const BUDGET_USD = [1, 2, 3, 5, 10, 20, 50, 0]

/** "08.00, 10.00, 14.00" — the chosen hours, sorted. */
export function hoursText(hours: number[]): string {
  return [...hours].sort((a, b) => a - b).map((h) => String(h).padStart(2, '0') + '.00').join(', ')
}

/** What Analisa WhatsApp otomatis does with this setting, in one sentence. */
export function autoText(a: WAAuto, providers: AIProvider[]): string {
  if (!a.enabled) return 'Mati — Analisa WhatsApp hanya dari routine Claude (claude.ai) dan tombol Analisa.'
  const p = providers.find((x) => x.id === a.engine)
  const label = p?.label ?? PROVIDER_LABEL[a.engine]
  if (!p?.key_source) return `${label} dipilih, tetapi kuncinya belum diisi — jadwal akan gagal sampai kunci diisi.`
  if (a.hours.length === 0) return 'Pilih minimal satu jam.'
  const days = a.weekend ? 'setiap hari' : 'Senin–Sabtu'
  const model = p.models.find((m) => m.id === a.model)?.label || a.model
  return `Aktif — ${days} jam ${hoursText(a.hours)} WIB dengan ${label} (${model}). Bila gagal, tidak diulang dengan AI lain; menunggu jam berikutnya.`
}
