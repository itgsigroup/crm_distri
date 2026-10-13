import { describe, expect, it } from 'vitest'
import { contactName, formatPairCode, historyAllowed } from '../src/session.js'

describe('session helpers', () => {
  it('formats the pairing code as WhatsApp shows it', () => {
    expect(formatPairCode('ABCD1234')).toBe('ABCD-1234')
    expect(formatPairCode('ABC')).toBe('ABC')
  })
  it('paces on-demand history requests per number', () => {
    const now = 10_000_000
    expect(historyAllowed([], now)).toBe(true)
    expect(historyAllowed([now - 1_000], now)).toBe(false)
    expect(historyAllowed([now - 5_000], now)).toBe(true)
    const hour = Array.from({ length: 400 }, (_, i) => now - 3_000_000 + i * 5_000)
    expect(historyAllowed(hour, now + 10_000)).toBe(false)
    expect(historyAllowed(hour.slice(1), now + 10_000)).toBe(true)
  })
  it('names a contact: saved name, then business name, then push name', () => {
    expect(contactName({ name: 'Pak Budi', notify: 'budi' })).toBe('Pak Budi')
    expect(contactName({ verifiedName: 'CV Sinar', notify: 'admin' })).toBe('CV Sinar')
    expect(contactName({ notify: ' budi ' })).toBe('budi')
    expect(contactName(undefined)).toBe('')
  })
})
