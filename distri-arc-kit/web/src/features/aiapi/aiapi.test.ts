import { describe, expect, it } from 'vitest'
import { modelLabel } from '../claude/usage'
import { byProvider, dailyBars, fallbackText, modelOption, providerOf, usd, type AIProvider } from './aiapi'

const providers: AIProvider[] = [
  { id: 'claude', label: 'Claude API', key_source: '', key_hint: '', model: 'claude-opus-5-5', models: [] },
  { id: 'openai', label: 'ChatGPT API', key_source: 'ui', key_hint: 'sk-proj…wxyz', model: 'gpt-5', models: [] },
]

describe('API & model AI', () => {
  it('maps llm_calls providers', () => {
    expect(providerOf('anthropic')).toBe('claude')
    expect(providerOf('openai')).toBe('openai')
    expect(providerOf('fake')).toBe('other')
  })

  it('sums cost per provider, largest first', () => {
    const rows = [
      { provider: 'anthropic', model: 'claude-opus-5-5', calls: 2, tokens_in: 100, tokens_out: 10, cost_idr: 300, last_at: '' },
      { provider: 'openai', model: 'gpt-5', calls: 1, tokens_in: 50, tokens_out: 5, cost_idr: 900, last_at: '' },
      { provider: 'anthropic', model: 'claude-haiku-4-5', calls: 1, tokens_in: 20, tokens_out: 2, cost_idr: 100, last_at: '' },
    ]
    expect(byProvider(rows)).toEqual([
      { provider: 'openai', calls: 1, tokens_in: 50, tokens_out: 5, cost_idr: 900 },
      { provider: 'claude', calls: 3, tokens_in: 120, tokens_out: 12, cost_idr: 400 },
    ])
  })

  it('keeps empty days on the chart and splits each day by provider', () => {
    const bars = dailyBars([
      { day: '2026-10-15', provider: 'anthropic', calls: 1, tokens_in: 1, tokens_out: 1, cost_idr: 200 },
      { day: '2026-10-15', provider: 'openai', calls: 1, tokens_in: 1, tokens_out: 1, cost_idr: 50 },
      { day: '2026-10-01', provider: 'openai', calls: 1, tokens_in: 1, tokens_out: 1, cost_idr: 999 },
    ], '2026-10-15', 3)
    expect(bars.map((b) => b.day)).toEqual(['2026-10-13', '2026-10-14', '2026-10-15'])
    expect(bars[2]).toEqual({ day: '2026-10-15', claude: 200, openai: 50, other: 0, total: 250 })
    expect(bars[0].total).toBe(0)
  })

  it('shows costs in dollars', () => {
    expect(usd(0, 16500)).toBe('$0')
    expect(usd(50, 16500)).toBe('<$0.01')
    expect(usd(16500 * 12.3, 16500)).toBe('$12.30')
    expect(usd(16500 * 1250, 16500)).toBe('$1,250')
  })

  it('lists a model with its price per million tokens', () => {
    expect(modelOption({ id: 'claude-opus-5-5', label: 'Claude Opus 5.5', in_usd: 4, out_usd: 20, live: false })).toBe('Claude Opus 5.5 · $4 / $20')
    expect(modelOption({ id: 'claude-haiku-5-5', label: 'Claude Haiku 5.5', in_usd: 0.1, out_usd: 0.5, live: true })).toBe('Claude Haiku 5.5 · $0.10 / $0.50')
    expect(modelOption({ id: 'gpt-5.1', label: '', in_usd: null, out_usd: null, live: true })).toBe('gpt-5.1 · harga ?')
  })

  it('names ChatGPT models', () => {
    expect(modelLabel('gpt-5-mini')).toBe('GPT-5 mini')
    expect(modelLabel('gpt-4.1')).toBe('GPT-4.1')
  })

  it('says what the second option does', () => {
    const base = { providers, mcp_wait_sec: 600, daily_budget_idr: 50000, spent_today_idr: 0 }
    expect(fallbackText({ ...base, fallback: 'off', ready: false })).toMatch(/^Mati/)
    expect(fallbackText({ ...base, fallback: 'claude', ready: false })).toMatch(/kuncinya belum diisi/)
    expect(fallbackText({ ...base, fallback: 'openai', ready: true })).toBe('Aktif — bila Claude lewat MCP belum menjawab dalam 10 menit, ChatGPT API (gpt-5) di server mengerjakan sisanya.')
    expect(fallbackText({ ...base, fallback: 'openai', ready: false, spent_today_idr: 50000 })).toMatch(/anggaran harian/)
  })
})
