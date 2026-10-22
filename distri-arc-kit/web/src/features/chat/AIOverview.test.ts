import { describe, expect, it } from 'vitest'
import { DECISION, provenance } from './AIOverview'

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
