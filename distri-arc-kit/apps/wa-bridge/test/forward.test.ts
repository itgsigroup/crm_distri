import { mkdtemp } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import pino from 'pino'
import { describe, expect, it } from 'vitest'
import { Forwarder } from '../src/forward.js'

describe('Forwarder', () => {
  it('drops a queued status older than one already delivered (no stale "unlinked" after re-pairing)', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'fwd-'))
    let up = false
    const got: unknown[] = []
    const fetchImpl = (async (_url: string, init?: RequestInit) => {
      if (!up) throw new Error('worker down')
      got.push(JSON.parse(String(init?.body)))
      return new Response('{"ok":true}', { status: 200 })
    }) as typeof fetch
    const f = new Forwarder('http://worker', 'secret-1234567890', dir, pino({ level: 'silent' }), fetchImpl)
    await f.send({ status: { session: '62812', status: 'disconnected', phone: '', qr: '', reason: 'unlinked', at: '2026-10-06T08:00:00.000Z' } })
    expect(await f.queueLength()).toBe(1)
    up = true
    await f.send({ status: { session: '62812', status: 'pairing', phone: '', qr: 'QR', at: '2026-10-06T08:05:00.000Z' } })
    await f.flush()
    expect(got).toHaveLength(1)
    expect((got[0] as { status: { status: string } }).status.status).toBe('pairing')
    expect(await f.queueLength()).toBe(0)
  })
})
