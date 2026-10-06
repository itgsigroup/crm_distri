import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import type { Policy } from './config.js'
import { dailyCap } from './guard.js'
import { SendError, type Sender, type SendTarget } from './sender.js'
import { verify } from './sign.js'
import type { Store } from './store.js'

/** The parts of a session the HTTP layer uses (lets tests run without WhatsApp). */
export interface BridgeSession extends SendTarget {
  snapshot(): { session: string; status: string; phone: string; qr: string; reason?: string }
  listGroups(): Promise<unknown[]>
  profile(jid: string): Promise<unknown>
}

export interface BridgeManager {
  get(id: string): BridgeSession | undefined
  statuses(): Record<string, string>
  create(id: string, label: string, historyDays: number): Promise<unknown>
  remove(id: string): Promise<boolean>
}

export interface Deps {
  secret: string
  manager: BridgeManager
  sender: Sender
  store: Store
  policy: Policy
  queueLength(): Promise<number>
  lastEvent(): Date | null
}

const SESSION_ID = /^[A-Za-z0-9_-]{1,64}$/

function json(res: ServerResponse, code: number, body: unknown, headers: Record<string, string> = {}) {
  res.writeHead(code, { 'Content-Type': 'application/json', ...headers })
  res.end(JSON.stringify(body))
}

async function readBody(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = []
  let size = 0
  for await (const c of req) {
    size += (c as Buffer).length
    if (size > 1 << 20) throw new Error('body terlalu besar')
    chunks.push(c as Buffer)
  }
  return Buffer.concat(chunks).toString('utf8')
}

/**
 * Endpoints (all except /health require X-ARC-Signature = HMAC-SHA256(BRIDGE_SECRET, raw body)):
 *   POST   /sessions                      {id, label, history_days} → {session, status, qr}
 *   GET    /sessions/{id}/qr              current QR / status
 *   GET    /sessions/{id}/groups          joined groups with members
 *   GET    /sessions/{id}/contacts/{jid}  profile (name, about, has_photo, business)
 *   POST   /sessions/{id}/send            {chat_id, text, action_id} → {wamid}
 *   DELETE /sessions/{id}                 log out and forget the device
 *   GET    /health                        {sessions, queue, last_event, limits}
 */
export function handler(d: Deps) {
  return async (req: IncomingMessage, res: ServerResponse) => {
    try {
      const url = new URL(req.url ?? '/', 'http://bridge')
      const parts = url.pathname.split('/').filter(Boolean).map(decodeURIComponent)
      const method = req.method ?? 'GET'

      if (method === 'GET' && url.pathname === '/health') {
        const now = new Date()
        const limits: Record<string, unknown> = {}
        for (const id of Object.keys(d.manager.statuses())) {
          const st = await d.store.stats(id, now)
          const meta = (await d.store.sessions()).find(m => m.id === id)
          const { cap, warmup } = dailyCap(d.policy, meta?.pairedAt ?? null, now)
          limits[id] = { last_hour: st.lastHour, per_hour: d.policy.maxPerHour, today: st.today, per_day: cap, warmup }
        }
        return json(res, 200, { ok: true, sessions: d.manager.statuses(), queue: await d.queueLength(), last_event: d.lastEvent()?.toISOString() ?? null, limits })
      }

      const body = await readBody(req)
      if (!verify(d.secret, body, req.headers['x-arc-signature'] as string | undefined)) return json(res, 401, { error: 'signature tidak valid' })

      if (parts[0] !== 'sessions') return json(res, 404, { error: 'tidak ditemukan' })

      if (method === 'POST' && parts.length === 1) {
        const inp = JSON.parse(body || '{}') as { id?: string; label?: string; history_days?: number }
        if (!inp.id || !SESSION_ID.test(inp.id)) return json(res, 400, { error: 'id sesi tidak valid' })
        return json(res, 200, await d.manager.create(inp.id, inp.label ?? inp.id, Number(inp.history_days ?? 30)))
      }

      const id = parts[1]
      if (method === 'DELETE' && parts.length === 2) {
        return (await d.manager.remove(id)) ? json(res, 200, { ok: true }) : json(res, 404, { error: 'sesi tidak ditemukan' })
      }
      const s = d.manager.get(id)
      if (!s) return json(res, 404, { error: 'sesi tidak ditemukan' })

      if (method === 'GET' && parts[2] === 'qr') return json(res, 200, s.snapshot())
      if (method === 'GET' && parts[2] === 'groups') {
        if (!s.connected()) return json(res, 409, { error: 'sesi belum terhubung' })
        return json(res, 200, await s.listGroups())
      }
      if (method === 'GET' && parts[2] === 'contacts' && parts[3]) return json(res, 200, await s.profile(parts[3]))
      if (method === 'POST' && parts[2] === 'send') {
        const inp = JSON.parse(body || '{}') as { chat_id?: string; text?: string; action_id?: string }
        try {
          const out = await d.sender.send(s, inp.chat_id ?? '', inp.text ?? '', inp.action_id ?? '')
          return json(res, 200, out)
        } catch (e) {
          if (e instanceof SendError) {
            return json(res, e.status, { error: e.message, code: e.code }, e.retryAfterSec ? { 'Retry-After': String(e.retryAfterSec) } : {})
          }
          throw e
        }
      }
      return json(res, 404, { error: 'tidak ditemukan' })
    } catch (e) {
      return json(res, 502, { error: e instanceof Error ? e.message : String(e) })
    }
  }
}

export function serve(d: Deps, port: number, host = '127.0.0.1'): Server {
  return createServer(handler(d)).listen(port, host)
}
