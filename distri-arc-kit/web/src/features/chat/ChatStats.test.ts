import { describe, expect, it } from 'vitest'
import { minutes, monthLabel } from './ChatStats'

describe('Statistik labels', () => {
  it('reads minutes as menit, jam or hari', () => {
    expect(minutes(null)).toBe('—')
    expect(minutes(0)).toBe('1 mnt')
    expect(minutes(45)).toBe('45 mnt')
    expect(minutes(150)).toBe('3 jam')
    expect(minutes(3 * 24 * 60)).toBe('3 hari')
  })
  it('names a month', () => {
    expect(monthLabel('2026-10')).toBe('Okt')
    expect(monthLabel('2026-01')).toBe('Jan')
  })
})
