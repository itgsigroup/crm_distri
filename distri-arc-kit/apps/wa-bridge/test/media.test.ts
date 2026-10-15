import { describe, expect, it } from 'vitest'
import { Readable } from 'node:stream'
import type { WAMessage } from 'baileys'
import { downloadWithFallback, fetchFailure, MEDIA_HOST } from '../src/media.js'

const msg = { key: { id: 'M1' } } as WAMessage
const netErr = () => Object.assign(new TypeError('fetch failed'), { cause: { code: 'ECONNRESET' } })

describe('media download fallback', () => {
  it('tells network failures from HTTP answers', () => {
    expect(fetchFailure(netErr())).toBe('ECONNRESET')
    expect(fetchFailure(Object.assign(new Error('x'), { status: 404 }))).toBe('')
    expect(fetchFailure('nope')).toBe('')
  })

  it('tries the default host, then a fresh copy, after a network failure', async () => {
    const calls: string[] = []
    const warns: string[] = []
    const fresh = { key: { id: 'M1-fresh' } } as WAMessage
    const out = await downloadWithFallback(msg, async (m, o) => {
      calls.push(`${m.key.id}@${o.host ?? 'stored'}`)
      if (m.key.id === 'M1') throw netErr()
      return Readable.from(['ok'])
    }, async () => fresh, (_, w) => warns.push(w))
    expect(calls).toEqual(['M1@stored', `M1@${MEDIA_HOST}`, `M1-fresh@${MEDIA_HOST}`])
    expect(warns).toHaveLength(2)
    expect(out).toBeDefined()
  })

  it('stops at the default host when it answers, and passes HTTP errors through untouched', async () => {
    let n = 0
    await downloadWithFallback(msg, async () => { if (n++ === 0) throw netErr(); return Readable.from(['ok']) }, async () => { throw new Error('no reupload') }, () => {})
    expect(n).toBe(2)
    const gone = Object.assign(new Error('gone'), { status: 410 })
    await expect(downloadWithFallback(msg, async () => { throw gone }, async () => msg, () => {})).rejects.toBe(gone)
  })
})
