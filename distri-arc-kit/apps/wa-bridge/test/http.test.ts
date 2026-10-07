import { mkdtemp } from 'node:fs/promises'
import { createServer, type Server } from 'node:http'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import pino from 'pino'
import { afterEach, describe, expect, it } from 'vitest'
import { defaultPolicy } from '../src/config.js'
import { Forwarder } from '../src/forward.js'
import { handler, type BridgeManager, type BridgeSession } from '../src/http.js'
import { Sender } from '../src/sender.js'
import { sign, verify } from '../src/sign.js'
import { MemoryStore } from '../src/store.js'

const SECRET = 's3cret-s3cret-123'
const log = pino({ level: 'silent' })
const servers: Server[] = []
afterEach(() => { for (const s of servers.splice(0)) s.close() })

async function listen(h: Parameters<typeof createServer>[1]): Promise<string> {
  const s = createServer(h)
  servers.push(s)
  await new Promise<void>(r => s.listen(0, '127.0.0.1', () => r()))
  const a = s.address() as { port: number }
  return `http://127.0.0.1:${a.port}`
}

describe('bridge HTTP', () => {
  it('rejects bad signatures and checks approval with the API before sending', async () => {
    // Fake ARC API: only action "ok-1" is approved, and the check itself must be signed.
    const api = await listen((req, res) => {
      const id = decodeURIComponent((req.url ?? '').replace('/bridge/actions/', ''))
      if (!verify(SECRET, id, req.headers['x-arc-signature'] as string)) { res.writeHead(401).end(); return }
      res.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({ approved: id === 'ok-1' }))
    })
    const store = new MemoryStore()
    await store.markInbound('s1', '6281@s.whatsapp.net', new Date(), false)
    const sent: string[] = []
    const session: BridgeSession = {
      id: 's1', connected: () => true, sendText: async (_c, text) => { sent.push(text); return 'W1' },
      snapshot: () => ({ session: 's1', status: 'connected', phone: '628', qr: '' }), listGroups: async () => [], profile: async () => ({}),
    }
    const manager: BridgeManager = { get: id => (id === 's1' ? session : undefined), statuses: () => ({ s1: 'connected' }), create: async (id, _l, _d, code) => ({ session: id, status: 'pairing', qr: code ? 'code:ABCD-1234' : '2@qr' }), remove: async () => true }
    const fwd = new Forwarder(api, SECRET, await mkdtemp(join(tmpdir(), 'br-')), log)
    const policy = { ...defaultPolicy, quietStartHour: 0, quietEndHour: 0, typingMinMs: 0, typingMaxMs: 0 }
    const bridge = await listen(handler({ secret: SECRET, manager, sender: new Sender(store, fwd, policy), store, policy, queueLength: async () => 0, lastEvent: () => null }))

    const post = (path: string, body: object, sig?: string) => {
      const raw = JSON.stringify(body)
      return fetch(bridge + path, { method: 'POST', body: raw, headers: { 'X-ARC-Signature': sig ?? sign(SECRET, raw) } })
    }
    expect((await post('/sessions/s1/send', { chat_id: '6281@s.whatsapp.net', text: 'Halo', action_id: 'ok-1' }, 'bad')).status).toBe(401)
    expect((await post('/sessions/s1/send', { chat_id: '6281@s.whatsapp.net', text: 'Halo', action_id: 'proposed-2' })).status).toBe(403)
    expect((await post('/sessions/zz/send', { chat_id: '6281@s.whatsapp.net', text: 'Halo', action_id: 'ok-1' })).status).toBe(404)
    expect(sent).toHaveLength(0)
    const ok = await post('/sessions/s1/send', { chat_id: '6281@s.whatsapp.net', text: 'Halo', action_id: 'ok-1' })
    expect(ok.status).toBe(200)
    expect(await ok.json()).toEqual({ wamid: 'W1', duplicate: false })
    expect(await (await post('/sessions', { id: '6281234567890', label: 'Andi', phone_code: true })).json()).toMatchObject({ qr: 'code:ABCD-1234' })
    expect((await post('/sessions', { id: 'cs-kantor', label: 'CS', phone_code: true })).status).toBe(400)
    expect(await (await post('/sessions', { id: 'link-ab12cd', label: 'Distri ARC', phone_code: true, phone: '0812-3456-7890' })).json()).toMatchObject({ qr: 'code:ABCD-1234' })
    const health = await (await fetch(bridge + '/health')).json()
    expect(health).toMatchObject({ ok: true, sessions: { s1: 'connected' }, limits: { s1: { last_hour: 1, per_hour: 20, today: 1 } } })
  })

  it('queues events while the API is down and flushes them in order, signed', async () => {
    let up = false
    const got: string[] = []
    const api = await listen(async (req, res) => {
      let raw = ''
      for await (const c of req) raw += c
      if (!up) { res.writeHead(503).end(); return }
      if (!verify(SECRET, raw, req.headers['x-arc-signature'] as string)) { res.writeHead(401).end(); return }
      got.push(JSON.parse(raw).event.wamid)
      res.writeHead(200).end('{}')
    })
    const fwd = new Forwarder(api, SECRET, await mkdtemp(join(tmpdir(), 'br-')), log)
    await fwd.send({ event: { wamid: 'w1' } })
    await fwd.send({ event: { wamid: 'w2' } })
    expect(await fwd.queueLength()).toBe(2)
    up = true
    await fwd.flush()
    expect(await fwd.queueLength()).toBe(0)
    expect(got).toEqual(['w1', 'w2'])
  })
})
