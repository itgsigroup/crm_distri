import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import type { Policy } from './config.js'
import { dailyCap } from './guard.js'
import { SendError, type Quote, type Sender, type SendTarget } from './sender.js'
import { HistoryBusy, MediaBusy } from './session.js'
import { ContentError, MAX_CONTENT_BYTES, type Content } from './content.js'
import { verify } from './sign.js'
import type { Store } from './store.js'

/** The members a send tags ("@Budi" in the text as "@<number>"): their JIDs; numbers become phone JIDs. */
export function mentionJids(v: unknown): string[] | undefined {
  if (!Array.isArray(v)) return undefined
  const out = v.filter((x): x is string => typeof x === 'string' && x.trim() !== '').map((x) => (x.includes('@') ? x : x.replace(/\D/g, '') + '@s.whatsapp.net'))
  return out.length ? [...new Set(out)] : undefined
}

/** The parts of a session the HTTP layer uses (lets tests run without WhatsApp). */
export interface BridgeSession extends SendTarget {
  snapshot(): { session: string; status: string; phone: string; qr: string; reason?: string }
  listGroups(): Promise<unknown[]>
  profile(jid: string): Promise<unknown>
  fetchHistory(chat: string, oldest: { id: string; fromMe: boolean; ts: number }, count: number): Promise<void>
  downloadMedia(ref: string): Promise<NodeJS.ReadableStream>
  /** Saves a number with a name in this account's WhatsApp contacts (synced to the phone). */
  saveContact(phone: string, fullName: string): Promise<void>
}

export interface BridgeManager {
  get(id: string): BridgeSession | undefined
  statuses(): Record<string, string>
  create(id: string, label: string, historyDays: number, phoneCode?: boolean, phone?: string): Promise<unknown>
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

async function readBody(req: IncomingMessage, limit = 1 << 20): Promise<string> {
  const chunks: Buffer[] = []
  let size = 0
  for await (const c of req) {
    size += (c as Buffer).length
    if (size > limit) throw new Error('body terlalu besar')
    chunks.push(c as Buffer)
  }
  return Buffer.concat(chunks).toString('utf8')
}

/**
 * Endpoints (all except /health require X-ARC-Signature = HMAC-SHA256(BRIDGE_SECRET, raw body)):
 *   POST   /sessions                      {id, label, history_days, phone_code?, phone?} → {session, status, qr}
 *                                         (phone_code: qr = "code:XXXX-XXXX" to type on the phone of `phone`)
 *   GET    /sessions/{id}/qr              current QR / status
 *   GET    /sessions/{id}/groups          joined groups with members
 *   GET    /sessions/{id}/contacts/{jid}  profile (name, about, has_photo, business)
 *   POST   /sessions/{id}/send            {chat_id, text, action_id, quote?: {id, from_me, participant?, text}, mentions?: [jid],
 *                                          content?: {kind: image|video|document|audio|poll, mime, file_name, data (base64),
 *                                          ptt, seconds, poll: {name, options, multi}}} → {wamid, ref?}
 *                                          live?: a person typing in Chat — skips the volume caps (ADR 0064)
 *   POST   /sessions/{id}/edit            {chat_id, wamid, text, action_id} → {wamid}: edit a sent text (approved action)
 *   POST   /sessions/{id}/delete          {chat_id, wamid, action_id} → {wamid}: delete a sent message for everyone (approved action)
 *   POST   /sessions/{id}/media           {ref, mime} → the file (streamed, never stored); 409 not connected,
 *                                         429 too many at once, 410 gone from WhatsApp and the phone
 *   POST   /sessions/{id}/history         {chat_id, id, from_me, ts (ms), count ≤ 50} → 202; the phone's answer
 *                                         arrives as history events (409 not connected, 429 paced)
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

      // a send may carry a file (base64, ≤ 25 MB raw); every other request stays small
      const body = await readBody(req, url.pathname.endsWith('/send') ? Math.ceil(MAX_CONTENT_BYTES * 1.4) + (1 << 20) : 1 << 20)
      if (!verify(d.secret, body, req.headers['x-arc-signature'] as string | undefined)) return json(res, 401, { error: 'signature tidak valid' })

      if (parts[0] !== 'sessions') return json(res, 404, { error: 'tidak ditemukan' })

      if (method === 'POST' && parts.length === 1) {
        const inp = JSON.parse(body || '{}') as { id?: string; label?: string; history_days?: number; phone_code?: boolean; phone?: string }
        if (!inp.id || !SESSION_ID.test(inp.id)) return json(res, 400, { error: 'id sesi tidak valid' })
        const phone = (inp.phone ?? (/^\d+$/.test(inp.id) ? inp.id : '')).replace(/\D/g, '')
        if (inp.phone_code && !/^\d{10,15}$/.test(phone)) return json(res, 400, { error: 'kode tautan butuh nomor telepon' })
        return json(res, 200, await d.manager.create(inp.id, inp.label ?? inp.id, Number(inp.history_days ?? 30), !!inp.phone_code, phone))
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
      if (method === 'POST' && parts[2] === 'contacts') {
        // a person saved a name for a number in GSI Orbit: it goes to this account's WhatsApp contacts (and the phone)
        const inp = JSON.parse(body || '{}') as { phone?: string; name?: string }
        const phone = (inp.phone ?? '').replace(/\D/g, '')
        const name = (inp.name ?? '').replace(/\s+/g, ' ').trim()
        if (!/^\d{8,15}$/.test(phone) || !name || name.length > 80) return json(res, 400, { error: 'nomor (8–15 digit) dan nama wajib', code: 'bad_request' })
        if (!s.connected()) return json(res, 409, { error: 'sesi belum terhubung', code: 'not_linked' })
        await s.saveContact(phone, name)
        return json(res, 200, { ok: true })
      }
      if (method === 'POST' && parts[2] === 'send') {
        const inp = JSON.parse(body || '{}') as { chat_id?: string; text?: string; action_id?: string; quote?: Quote; content?: Content; mentions?: string[]; live?: boolean }
        try {
          const out = await d.sender.send(s, inp.chat_id ?? '', inp.text ?? '', inp.action_id ?? '', inp.quote?.id ? inp.quote : undefined, inp.content?.kind ? inp.content : undefined, mentionJids(inp.mentions), inp.live === true)
          return json(res, 200, out)
        } catch (e) {
          if (e instanceof ContentError) return json(res, 400, { error: e.message, code: 'bad_content' })
          if (e instanceof SendError) {
            return json(res, e.status, { error: e.message, code: e.code }, e.retryAfterSec ? { 'Retry-After': String(e.retryAfterSec) } : {})
          }
          throw e
        }
      }
      if (method === 'POST' && parts[2] === 'edit') {
        const inp = JSON.parse(body || '{}') as { chat_id?: string; wamid?: string; text?: string; action_id?: string }
        try {
          return json(res, 200, await d.sender.edit(s, inp.chat_id ?? '', inp.wamid ?? '', inp.text ?? '', inp.action_id ?? ''))
        } catch (e) {
          if (e instanceof SendError) return json(res, e.status, { error: e.message, code: e.code })
          throw e
        }
      }
      if (method === 'POST' && parts[2] === 'delete') {
        const inp = JSON.parse(body || '{}') as { chat_id?: string; wamid?: string; action_id?: string }
        try {
          return json(res, 200, await d.sender.revoke(s, inp.chat_id ?? '', inp.wamid ?? '', inp.action_id ?? ''))
        } catch (e) {
          if (e instanceof SendError) return json(res, e.status, { error: e.message, code: e.code })
          throw e
        }
      }
      if (method === 'POST' && parts[2] === 'media') {
        const inp = JSON.parse(body || '{}') as { ref?: string; mime?: string }
        if (!inp.ref) return json(res, 400, { error: 'ref wajib' })
        let stream: NodeJS.ReadableStream
        try {
          stream = await s.downloadMedia(inp.ref)
        } catch (e) {
          if (e instanceof MediaBusy) return json(res, e.status, { error: e.message, code: e.status === 409 ? 'not_linked' : 'busy' })
          const status = (e as { output?: { statusCode?: number }; status?: number }).output?.statusCode ?? (e as { status?: number }).status
          if (status === 404 || status === 410) return json(res, 410, { error: 'media sudah tidak tersedia di WhatsApp', code: 'gone' })
          throw e
        }
        res.writeHead(200, { 'Content-Type': /^[\w.+-]+\/[\w.+-]+/.test(inp.mime ?? '') ? (inp.mime as string).split(';')[0] : 'application/octet-stream' })
        for await (const chunk of stream) {
          if (!res.write(chunk)) await new Promise(r => res.once('drain', r))
        }
        res.end()
        return
      }
      if (method === 'POST' && parts[2] === 'history') {
        const inp = JSON.parse(body || '{}') as { chat_id?: string; id?: string; from_me?: boolean; ts?: number; count?: number }
        if (!inp.chat_id || !inp.id || !Number.isFinite(inp.ts)) return json(res, 400, { error: 'chat_id, id dan ts wajib' })
        try {
          await s.fetchHistory(inp.chat_id, { id: inp.id, fromMe: !!inp.from_me, ts: Number(inp.ts) }, Number(inp.count ?? 50))
          return json(res, 202, { ok: true })
        } catch (e) {
          if (e instanceof HistoryBusy) return json(res, e.status, { error: e.message, code: e.status === 409 ? 'not_linked' : 'paced' })
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
