import { describe, expect, it } from 'vitest'
import { monthLabel } from './ChatBackupCard'

describe('monthLabel', () => {
  it('names a backup month in Indonesian', () => {
    expect(monthLabel('2026-10')).toBe('Oktober 2026')
    expect(monthLabel('2027-01')).toBe('Januari 2027')
    expect(monthLabel('x')).toBe('x')
  })
})
