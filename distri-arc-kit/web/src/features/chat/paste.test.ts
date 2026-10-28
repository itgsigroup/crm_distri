import { describe, expect, it } from 'vitest'
import { fromSpreadsheet, pastedImage } from './paste'

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
  it('cells copied from Excel go as the picture of the table, like WhatsApp Web', () => {
    const excel = '<html xmlns:x="urn:schemas-microsoft-com:office:excel"><body><table><tr><td>DM CCTV CIBINONG</td><td>13.222.358</td></tr></table></body></html>'
    const f = pastedImage([png('image.png')], 'DM CCTV CIBINONG\t13.222.358\r\n', new Date('2026-10-28T05:00:00Z'), excel)
    expect(f?.name).toBe('tabel-20261028-050000.png')
    // tab-separated rows with an HTML table (another spreadsheet) count too
    expect(fromSpreadsheet('<table><tr><td>a</td></tr></table>', 'a\tb')).toBe(true)
    // Excel cells without a picture (some browsers): the text, as before
    expect(pastedImage([], 'DM CCTV\t13.222.358', new Date(), excel)).toBeNull()
    // a web page or a Word document with a picture: still text
    expect(pastedImage([png('image.png')], 'Harga kamera 4MP', new Date(), '<html><meta name=ProgId content=Word.Document><p>Harga kamera 4MP</p></html>')).toBeNull()
  })
})
