import { beforeEach, describe, expect, it, vi } from 'vitest'

// Node's own localStorage (no file) has no methods and hides jsdom's: a plain store
const m = new Map<string, string>()
vi.stubGlobal('localStorage', { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k) })

describe('chat drafts', () => {
  beforeEach(() => m.clear())
  it('keeps a draft per chat until it is blank, and remembers it in the browser', async () => {
    const { getDraft, setDraft } = await import('./drafts')
    setDraft('a', 'halo pak, stok')
    setDraft('b', 'besok kirim')
    expect(getDraft('a')).toBe('halo pak, stok')
    expect(getDraft('b')).toBe('besok kirim')
    expect(JSON.parse(m.get('chat.drafts')!)).toEqual({ a: 'halo pak, stok', b: 'besok kirim' })
    setDraft('a', '   ')
    expect(getDraft('a')).toBe('')
    expect(JSON.parse(m.get('chat.drafts')!)).toEqual({ b: 'besok kirim' })
  })
})
