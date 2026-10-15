import { mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import pino from 'pino'
import { describe, expect, it } from 'vitest'
import { Forwarder, postTimeout } from '../src/forward.js'

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

  it('a batch left by a restart mid-flush is delivered, not overwritten by the next queue', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'fwd-'))
    const got: string[] = []
    const fetchImpl = (async (_url: string, init?: RequestInit) => {
      got.push((JSON.parse(String(init?.body)) as { event: { wamid: string } }).event.wamid)
      return new Response('{"ok":true}', { status: 200 })
    }) as typeof fetch
    // the previous process died while flushing A and B; C was queued since
    await writeFile(join(dir, 'queue.jsonl.flushing'), ['A', 'B'].map((w) => JSON.stringify({ event: { wamid: w } })).join('\n') + '\n')
    await writeFile(join(dir, 'queue.jsonl'), JSON.stringify({ event: { wamid: 'C' } }) + '\n')
    const f = new Forwarder('http://worker', 'secret-1234567890', dir, pino({ level: 'silent' }), fetchImpl)
    expect(await f.queueLength()).toBe(3)
    await f.flush()
    expect(got).toEqual(['A', 'B'])
    expect(await f.queueLength()).toBe(1)
    await f.flush()
    expect(got).toEqual(['A', 'B', 'C'])
    expect(await f.queueLength()).toBe(0)
    await expect(readFile(join(dir, 'queue.jsonl.flushing'))).rejects.toThrow()
  })

  it('a failed post keeps it and the rest of the batch queued in order', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'fwd-'))
    const got: string[] = []
    let calls = 0
    const fetchImpl = (async (_url: string, init?: RequestInit) => {
      if (++calls === 2) throw new Error('timeout')
      got.push((JSON.parse(String(init?.body)) as { event: { wamid: string } }).event.wamid)
      return new Response('{"ok":true}', { status: 200 })
    }) as typeof fetch
    await writeFile(join(dir, 'queue.jsonl.flushing'), ['A', 'B', 'C'].map((w) => JSON.stringify({ event: { wamid: w } })).join('\n') + '\n')
    const f = new Forwarder('http://worker', 'secret-1234567890', dir, pino({ level: 'silent' }), fetchImpl)
    await f.flush()
    expect(got).toEqual(['A'])
    expect(await f.queueLength()).toBe(2)
    await f.flush()
    expect(got).toEqual(['A', 'B', 'C'])
    expect(await f.queueLength()).toBe(0)
  })

  it('gives a large history batch more time than a live message, capped', () => {
    expect(postTimeout(2_000)).toBe(30_000)
    expect(postTimeout(5_700_000)).toBe(130_000)
    expect(postTimeout(50_000_000)).toBe(180_000)
  })
})
