import { describe, expect, it } from 'vitest'
import { RUN_COMPARE, aiAgents, argsText, waitText, dur, hourly, modelLabel, rpAI, tok, type AIRun } from './usage'

const run = (x: Partial<AIRun>): AIRun => ({ kind: 'cycle', id: '1', title: 'Siklus #1', trigger: 'schedule', by: '', via: 'api', status: 'done', started_at: '2026-10-10T01:00:00Z', duration_ms: 1000, model: 'claude-sonnet-5-5', calls: 2, tokens_in: 100, tokens_out: 20, cost_idr: 50, ...x })

describe('pemakaian AI', () => {
  it('formats cost, tokens and duration', () => {
    expect(rpAI(0)).toBe('Rp0')
    expect(rpAI(1250)).toBe('Rp1.250')
    expect(rpAI(1_250_000)).toBe('Rp1,3 jt')
    expect(tok(850)).toBe('850')
    expect(tok(12_400)).toBe('12,4 rb')
    expect(dur(1200)).toBe('1,2 dtk')
    expect(dur(185_000)).toBe('3 mnt 5 dtk')
    expect(dur(null)).toBe('—')
    expect(hourly(6, 20)).toBe('Tiap jam 06.00–20.00 WIB')
  })

  it('names models for people', () => {
    expect(modelLabel('claude-sonnet-5-5')).toBe('Claude Sonnet 5.5')
    expect(modelLabel('claude-opus-5-5, gpt-4.1')).toBe('Claude Opus 5.5, gpt-4.1')
    expect(modelLabel('template')).toBe('Template (tanpa AI)')
    expect(modelLabel('fake')).toBe('Template (tanpa AI)')
    expect(modelLabel('claude.ai')).toBe('Claude (akun claude.ai)')
    expect(modelLabel('none')).toBe('Tanpa AI — gagal')
    expect(modelLabel('claude-sonnet-5-5, claude.ai')).toBe('Claude Sonnet 5.5, Claude (akun claude.ai)')
  })

  it('sorts the history by cost and tokens', () => {
    expect(RUN_COMPARE.cost_idr(run({ cost_idr: 10 }), run({ cost_idr: 90 }))).toBeLessThan(0)
    expect(RUN_COMPARE.tokens(run({ tokens_in: 900 }), run({ tokens_in: 100 }))).toBeGreaterThan(0)
  })
})

describe('riwayat MCP', () => {
  it('shows call arguments short', () => {
    expect(argsText({ dealer_id: 'mitra', limit: 10 })).toBe('dealer_id=mitra, limit=10')
    expect(argsText(null)).toBe('')
  })
})

describe('wajib AI', () => {
  it('counts only agents a model analysed', () => {
    expect(aiAgents({ model: 'template', agents: 6, failed_agents: 0 })).toBe(0)
    expect(aiAgents({ model: 'none', agents: 6, failed_agents: 6 })).toBe(0)
    expect(aiAgents({ model: 'claude.ai', agents: 6, failed_agents: 5 })).toBe(1)
    expect(waitText(25)).toBe('25 detik')
    expect(waitText(600)).toBe('10 menit')
  })
})
