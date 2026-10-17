import { describe, expect, it } from 'vitest'
import { ordered, summarize, type ProgressAgent } from './progress'

const agents: ProgressAgent[] = [
  { agent: 'AI Kredit', state: 'done', covers: '3 dealer' },
  { agent: 'AI Stok', state: 'reading', covers: '1 dealer · 2 stok' },
  { agent: 'AI Order', state: 'waiting' },
  { agent: 'AI Penagihan', state: 'failed' },
]

describe('analysis progress', () => {
  it('says what the AI analyses now', () => {
    expect(summarize({ agents, steps: [] })).toEqual({ done: 2, total: 4, pct: 50, now: 'AI sedang menganalisa AI Stok (1 dealer · 2 stok)' })
    expect(summarize({ agents: [], steps: [{ at: '', tool: 'orchestrator_status', text: 'memeriksa status siklus' }] }).now).toBe('AI memeriksa status siklus')
    expect(summarize({ agents: [{ agent: 'AI Order', state: 'waiting' }], steps: [] }).now).toBe('Menunggu AI mulai menganalisa')
  })
  it('puts the agent being read first', () => {
    expect(ordered(agents).map((a) => a.agent)).toEqual(['AI Stok', 'AI Order', 'AI Penagihan', 'AI Kredit'])
  })
})
