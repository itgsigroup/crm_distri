import { describe, expect, it } from 'vitest'
import { linkParts } from './links'

describe('links in a bubble', () => {
  it('makes web addresses clickable, leaving the sentence around them', () => {
    expect(linkParts('cek https://gsi.co.id/katalog?x=1 ya')).toEqual([
      { t: 'cek ' }, { t: 'https://gsi.co.id/katalog?x=1', href: 'https://gsi.co.id/katalog?x=1' }, { t: ' ya' },
    ])
    expect(linkParts('lihat www.tokopedia.com/gsi.')).toEqual([
      { t: 'lihat ' }, { t: 'www.tokopedia.com/gsi', href: 'https://www.tokopedia.com/gsi' }, { t: '.' },
    ])
    expect(linkParts('(http://a.id/b) dan https://en.wikipedia.org/wiki/CCTV_(camera)')).toEqual([
      { t: '(' }, { t: 'http://a.id/b', href: 'http://a.id/b' }, { t: ') dan ' },
      { t: 'https://en.wikipedia.org/wiki/CCTV_(camera)', href: 'https://en.wikipedia.org/wiki/CCTV_(camera)' },
    ])
  })
  it('leaves text without web addresses alone', () => {
    expect(linkParts('harga 1.500.000, email budi@gsi.co.id')).toEqual([{ t: 'harga 1.500.000, email budi@gsi.co.id' }])
    expect(linkParts('javascript:alert(1)')).toEqual([{ t: 'javascript:alert(1)' }])
  })
})
