import { describe, expect, it } from 'vitest'
import { pastedImage } from './paste'

const png = (name: string) => new File([new Uint8Array([137, 80, 78, 71])], name, { type: 'image/png' })

describe('tempel di kolom chat', () => {
  it('a screenshot becomes a named picture to send', () => {
    const f = pastedImage([png('image.png')], '', new Date('2026-10-15T09:30:05Z'))
    expect(f?.name).toBe('gambar-20261015-093005.png')
    expect(f?.type).toBe('image/png')
  })
  it('a copied picture keeps its name', () => {
    expect(pastedImage([png('stok-gudang.png')], '')?.name).toBe('stok-gudang.png')
  })
  it('text stays text, even with a picture beside it', () => {
    expect(pastedImage([png('image.png')], 'Harga kamera 4MP')).toBeNull()
    expect(pastedImage([], 'halo')).toBeNull()
    expect(pastedImage([new File(['x'], 'a.pdf', { type: 'application/pdf' })], '')).toBeNull()
  })
})
