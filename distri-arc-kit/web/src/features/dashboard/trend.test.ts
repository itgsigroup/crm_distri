import { describe, expect, it } from 'vitest'
import { trendChange, trendHeads } from './trend'

describe('trendChange', () => {
  it('compares with the previous point that has data', () => {
    expect(trendChange([100, null, 120], 2)).toEqual({ dir: 'up', text: '+20%' })
    expect(trendChange([200, 150], 1)).toEqual({ dir: 'down', text: '−25%' })
  })
  it('has nothing to say for the first point, an empty point, or a start from zero', () => {
    expect(trendChange([5, 6], 0)).toBeNull()
    expect(trendChange([5, null], 1)).toBeNull()
    expect(trendChange([0, 3], 1)).toEqual({ dir: 'up', text: 'dari 0' })
    expect(trendChange([3, 3], 1)).toEqual({ dir: 'flat', text: 'sama' })
  })
})

describe('trendHeads', () => {
  const short = (iso: string) => `${Number(iso.slice(8))}/${Number(iso.slice(5, 7))}`
  it('names each day by its date', () => {
    expect(trendHeads('days', ['1', '2'], ['2026-10-01', '2026-10-02'], short)).toEqual(['1/10', '2/10'])
  })
  it('marks only a running month', () => {
    expect(trendHeads('months', ['Sep 26', 'Okt 26'], ['2026-09-30', '2026-10-05'], short)).toEqual(['Sep 26', 'Okt 26 · s.d. 5/10'])
    expect(trendHeads('months', ['Sep 26'], ['2026-09-30'], short)).toEqual(['Sep 26'])
  })
  it('a change smaller than one percent still has a direction', () => {
    expect(trendChange([1000, 999], 1)).toEqual({ dir: 'down', text: '−<1%' })
  })
})
