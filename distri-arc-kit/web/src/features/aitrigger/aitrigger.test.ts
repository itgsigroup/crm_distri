import { describe, expect, it } from 'vitest'
import { parseKeywords, unknownPlaceholders } from './aitrigger'

describe('AI Trigger helpers', () => {
  it('cleans keywords', () => {
    expect(parseKeywords('Stok, ready ,, ada barang\nstok')).toEqual(['stok', 'ready', 'ada barang'])
  })
  it('finds placeholders the kind does not fill', () => {
    expect(unknownPlaceholders('Harga {produk} Rp {harga} {jumlah}', 'price')).toEqual(['jumlah'])
    expect(unknownPlaceholders('Order {nomor} {status}', 'order')).toEqual([])
  })
})
