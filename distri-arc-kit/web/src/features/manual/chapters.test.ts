import { describe, expect, it } from 'vitest'
import { ROUTES } from '../../app/Shell'
import { CHAPTERS, searchChapters } from './chapters'

describe('Buku manual', () => {
  it('has unique chapters, each opening with its own title, each menu link a real page', () => {
    const ids = CHAPTERS.map((c) => c.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const c of CHAPTERS) {
      expect(c.md.trim().startsWith('# ')).toBe(true)
      if (c.screen) expect(ROUTES[c.screen]).toBeTruthy()
    }
  })
  it('covers every main menu', () => {
    const screens = new Set(CHAPTERS.map((c) => c.screen))
    for (const s of ['today', 'dashboard', 'orch', 'chat', 'reportwa', 'orbit', 'dealer', 'stock', 'stockcheck', 'ar', 'users', 'conn'] as const) expect(screens.has(s)).toBe(true)
  })
  it('finds chapters holding every word, any case', () => {
    expect(searchChapters('').length).toBe(CHAPTERS.length)
    expect(searchChapters('REORDER segera').map((c) => c.id)).toContain('cek-stok')
    expect(searchChapters('label pengirim').map((c) => c.id)).toContain('chat')
    expect(searchChapters('zzzz tidak ada')).toEqual([])
  })
})
