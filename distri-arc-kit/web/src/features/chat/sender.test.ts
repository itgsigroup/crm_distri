import { describe, expect, it } from 'vitest'
import { senderInitial } from './ChatPage'

describe('sender label', () => {
  it('takes the first letter of the admin, past a title', () => {
    expect(senderInitial('Tantri')).toBe('T')
    expect(senderInitial('sekar ayu')).toBe('S')
    expect(senderInitial('Mbak Rina')).toBe('R')
    expect(senderInitial('  ')).toBe('?')
  })
})
