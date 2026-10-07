import { describe, expect, it } from 'vitest'
import { contactName, formatPairCode } from '../src/session.js'

describe('session helpers', () => {
  it('formats the pairing code as WhatsApp shows it', () => {
    expect(formatPairCode('ABCD1234')).toBe('ABCD-1234')
    expect(formatPairCode('ABC')).toBe('ABC')
  })
  it('names a contact: saved name, then business name, then push name', () => {
    expect(contactName({ name: 'Pak Budi', notify: 'budi' })).toBe('Pak Budi')
    expect(contactName({ verifiedName: 'CV Sinar', notify: 'admin' })).toBe('CV Sinar')
    expect(contactName({ notify: ' budi ' })).toBe('budi')
    expect(contactName(undefined)).toBe('')
  })
})
