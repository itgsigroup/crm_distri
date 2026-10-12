import { describe, expect, it } from 'vitest'
import { bubbleLine } from './bubble'

describe('bubbleLine', () => {
  it('a customer order waiting too long', () => {
    const l = bubbleLine({ state: 'needs_reply', wait_min: 300, analyzed: true, intent: 'order', items: [{ name: 'Kamera IP 4MP', qty: 10 }] })
    expect(l).toEqual({ state: 'menunggu 5 jam — perlu dibalas', tone: 'bad', request: { label: 'Minta order', tone: 'good', items: '10 Kamera IP 4MP' } })
  })
  it('answered quickly, answered late, and the sales side', () => {
    expect(bubbleLine({ state: 'replied', wait_min: 25, analyzed: true, intent: 'ask_price' }).tone).toBe('good')
    expect(bubbleLine({ state: 'replied', wait_min: 200, analyzed: true }).state).toBe('dibalas 3 jam kemudian')
    expect(bubbleLine({ state: 'reply', wait_min: 45, analyzed: true, intent: 'none' })).toEqual({ state: 'membalas setelah pelanggan menunggu 45 menit', tone: 'good' })
    expect(bubbleLine({ state: 'follow_up', wait_min: null, analyzed: true }).state).toBe('sales menyapa lebih dulu')
  })
  it('a closing message and a bubble not analysed yet carry no request', () => {
    expect(bubbleLine({ state: 'closing', wait_min: null, analyzed: true, intent: 'other' }).request).toBeUndefined()
    expect(bubbleLine({ state: 'waiting', wait_min: 10, analyzed: false }).request).toBeUndefined()
  })
})
