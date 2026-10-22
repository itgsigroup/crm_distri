import { describe, expect, it } from 'vitest'
import { DECISION, latestReading, lineChip, provenance } from './AIOverview'

describe('Analisa Claude provenance', () => {
  it('names the cycle, the date and the confidence of a line', () => {
    const p = provenance(1204, '2026-10-12T03:00:00Z', 80)
    expect(p.startsWith('Siklus #1.204 · ')).toBe(true)
    expect(p.endsWith(' · yakin 80%')).toBe(true)
    expect(provenance(null, '2026-10-12T03:00:00Z', null)).not.toContain('Siklus')
  })
  it('labels every decision a person can make', () => {
    for (const s of ['proposed', 'approved', 'edited', 'executed', 'rejected', 'expired']) expect(DECISION[s]).toBeTruthy()
  })
})

describe('Analisa Claude timeline', () => {
  const at = '2026-10-10T09:00:00Z'
  it('leads with the newest reading of the chat, not a note on one message', () => {
    const tl = [
      { kind: 'summary' as const, cycle_number: 1, at, summary: 'awal', confidence: 80 },
      { kind: 'overview' as const, cycle_number: 2, at, summary: 'overview chat', confidence: null },
      { kind: 'note' as const, cycle_number: 2, at, summary: 'catatan', confidence: 90 },
    ]
    expect(latestReading(tl)?.summary).toBe('overview chat')
    expect(latestReading([tl[2]])?.summary).toBe('catatan')
    expect(latestReading([])).toBeNull()
  })
  it('chips a line with what the customer asked, else its kind', () => {
    expect(lineChip({ kind: 'summary', cycle_number: 1, at, summary: 'x', confidence: null, intent_label: 'Tanya stok' })[0]).toBe('Tanya stok')
    expect(lineChip({ kind: 'summary', cycle_number: 1, at, summary: 'x', confidence: null, intent: 'order' })[0]).toBe('Minta order')
    expect(lineChip({ kind: 'overview', cycle_number: 1, at, summary: 'x', confidence: null })[0]).toBe('Analisa chat ini')
  })
})
