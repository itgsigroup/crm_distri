import makeWASocket, {
  Browsers, DisconnectReason, fetchLatestBaileysVersion, isJidGroup, isLidUser, jidNormalizedUser, makeCacheableSignalKeyStore,
  type Contact, type GroupMetadata, type WASocket, type proto,
} from 'baileys'
import type { Logger } from 'pino'
import type { Policy } from './config.js'
import { isConversation, toGroupMeta, toWaEvent, type GroupMeta, type Resolver, type WaEvent } from './events.js'
import type { Forwarder } from './forward.js'
import { isOptOut, reconnectDelayMs } from './guard.js'
import type { PgStore } from './pgstore.js'
import type { SendTarget } from './sender.js'
import type { SessionMeta } from './store.js'

export type Status = 'pairing' | 'connected' | 'disconnected'

export interface Snapshot { session: string; status: Status; phone: string; qr: string; reason?: string }

export interface Profile { name: string; about: string; has_photo: boolean; business: string }

const GROUP_TTL = 60 * 60_000
const GROUP_LIST_TTL = 10 * 60_000
const PROFILE_TTL = 7 * 24 * 60 * 60_000
const STABLE_MS = 5 * 60_000
// Close codes after which reconnecting would only look abusive: stop and ask a human.
const FATAL = new Set<number>([DisconnectReason.loggedOut, DisconnectReason.forbidden, DisconnectReason.connectionReplaced, DisconnectReason.multideviceMismatch, DisconnectReason.badSession])

const digits = (jid?: string | null) => (jid ?? '').split('@')[0].split(':')[0].replace(/\D/g, '')

/**
 * One linked WhatsApp number. Connection hygiene to stay under WhatsApp's radar:
 * a stable device name, not marked online on connect (the phone keeps its
 * notifications), recent history only, cached group metadata, rate-limited
 * profile lookups, exponential reconnect backoff, and no reconnect after a
 * logout/ban/replacement (a human must re-link).
 */
export class Session implements SendTarget {
  sock?: WASocket
  status: Status = 'disconnected'
  qr = ''
  reason = ''
  private attempts = 0
  private stopped = false
  private openedAt = 0
  private timer?: NodeJS.Timeout
  private groups = new Map<string, { meta: GroupMetadata; at: number }>()
  private groupList?: { at: number; items: { jid: string; meta: GroupMeta }[] }
  private profiles = new Map<string, { at: number; p: Profile }>()
  private lookups: number[] = []
  private contacts = new Map<string, Partial<Contact>>()
  private recent = new Map<string, proto.IMessage>() // for retry receipts (getMessage)
  private waiters: ((s: Snapshot) => void)[] = []

  constructor(
    public meta: SessionMeta,
    private store: PgStore,
    private fwd: Forwarder,
    private policy: Policy,
    private log: Logger,
  ) {}

  get id(): string { return this.meta.id }

  snapshot(): Snapshot {
    const s: Snapshot = { session: this.meta.id, status: this.status, phone: this.meta.phone, qr: this.qr }
    if (this.reason) s.reason = this.reason
    return s
  }

  connected(): boolean { return this.status === 'connected' && !!this.sock }

  /** Resolves once the first QR (or an open connection) is available. */
  firstState(timeoutMs: number): Promise<Snapshot> {
    return new Promise(resolve => {
      const t = setTimeout(() => resolve(this.snapshot()), timeoutMs)
      this.waiters.push(s => { clearTimeout(t); resolve(s) })
    })
  }

  private setStatus(status: Status, reason = '') {
    this.status = status
    this.reason = reason
    if (status !== 'pairing') this.qr = ''
    const snap = this.snapshot()
    for (const w of this.waiters.splice(0)) w(snap)
    void this.fwd.send({ status: snap })
  }

  async start(): Promise<void> {
    this.stopped = false
    const { state, saveCreds } = await this.store.authState(this.meta.id)
    // Always speak the current WhatsApp Web version: outdated clients get flagged.
    const version = await fetchLatestBaileysVersion().then(v => v.version).catch(() => undefined)
    const sock = makeWASocket({
      auth: { creds: state.creds, keys: makeCacheableSignalKeyStore(state.keys, this.log) },
      ...(version ? { version } : {}),
      browser: Browsers.ubuntu('ARC'),
      logger: this.log,
      markOnlineOnConnect: false,
      syncFullHistory: false,
      emitOwnEvents: false,
      generateHighQualityLinkPreview: false,
      shouldIgnoreJid: jid => !isConversation(jid),
      cachedGroupMetadata: async jid => this.cachedGroup(jid),
      getMessage: async key => (key.id ? this.recent.get(key.id) : undefined),
    })
    this.sock = sock
    sock.ev.on('creds.update', () => void saveCreds())
    sock.ev.on('connection.update', u => void this.onConnection(u, state.creds.registered))
    sock.ev.on('messages.upsert', ({ messages, type }) => {
      if (type !== 'notify' && type !== 'append') return
      void this.forwardMessages(messages, false)
    })
    sock.ev.on('messaging-history.set', ({ messages }) => void this.forwardMessages(messages, true))
    sock.ev.on('contacts.upsert', cs => { for (const c of cs) this.contacts.set(c.id, { ...this.contacts.get(c.id), ...c }) })
    sock.ev.on('contacts.update', cs => { for (const c of cs) if (c.id) this.contacts.set(c.id, { ...this.contacts.get(c.id), ...c }) })
    sock.ev.on('groups.update', gs => { for (const g of gs) if (g.id) this.groups.delete(g.id); this.groupList = undefined })
    sock.ev.on('group-participants.update', g => { this.groups.delete(g.id); this.groupList = undefined })
  }

  private async onConnection(u: Partial<import('baileys').ConnectionState>, wasRegistered: boolean) {
    if (u.qr) {
      this.qr = u.qr
      this.setStatus('pairing')
    }
    if (u.connection === 'open') {
      this.openedAt = Date.now()
      this.meta.phone = digits(this.sock?.user?.id)
      if (!this.meta.pairedAt) this.meta.pairedAt = new Date()
      await this.store.upsertSession(this.meta)
      this.setStatus('connected')
      setTimeout(() => { if (this.status === 'connected' && Date.now() - this.openedAt >= STABLE_MS) this.attempts = 0 }, STABLE_MS + 1000)
    }
    if (u.connection === 'close') {
      const code = (u.lastDisconnect?.error as { output?: { statusCode?: number } } | undefined)?.output?.statusCode ?? 0
      this.sock = undefined
      if (this.stopped) return
      if (code === DisconnectReason.restartRequired) {
        void this.start() // normal right after pairing
        return
      }
      if (FATAL.has(code) || (!wasRegistered && this.status !== 'connected' && !this.meta.pairedAt)) {
        const reason = code === DisconnectReason.loggedOut ? 'logged_out' : code === DisconnectReason.forbidden ? 'forbidden' : code === DisconnectReason.connectionReplaced ? 'replaced' : code ? 'closed_' + code : 'pairing_ended'
        if (code === DisconnectReason.forbidden) this.log.error({ session: this.id }, 'WhatsApp refused the session (403) — number may be restricted; not reconnecting')
        if (code === DisconnectReason.loggedOut) await this.store.deleteSession(this.id).then(() => this.store.upsertSession({ ...this.meta, pairedAt: null }))
        this.stopped = true
        this.setStatus('disconnected', reason)
        return
      }
      const delay = reconnectDelayMs(this.attempts++)
      this.log.warn({ session: this.id, code, delay }, 'connection closed, reconnecting with backoff')
      this.setStatus('disconnected', 'reconnecting')
      clearTimeout(this.timer)
      this.timer = setTimeout(() => void this.start(), delay)
    }
  }

  async stop(logout: boolean): Promise<void> {
    this.stopped = true
    clearTimeout(this.timer)
    const sock = this.sock
    this.sock = undefined
    if (sock) {
      if (logout) await sock.logout().catch(() => {})
      else sock.end(undefined)
    }
  }

  private resolver(): Resolver {
    return {
      ownPhone: this.meta.phone || digits(this.sock?.user?.id),
      pnForLid: async lid => (await this.sock?.signalRepository.lidMapping.getPNForLID(lid).catch(() => null)) ?? null,
      groupMeta: async jid => {
        const g = await this.cachedGroup(jid)
        return g ? toGroupMeta(g.subject, g.participants) : undefined
      },
    }
  }

  private async forwardMessages(messages: import('baileys').WAMessage[], history: boolean) {
    const cutoff = Date.now() / 1000 - this.meta.historyDays * 86_400
    const r = this.resolver()
    const batch: WaEvent[] = []
    for (const m of messages) {
      const ts = Number(m.messageTimestamp ?? 0)
      const ev = await toWaEvent(this.id, m, r, history).catch(err => { this.log.warn({ err: String(err) }, 'map message failed'); return null })
      if (!ev) continue
      if (!ev.from_me) {
        // Inbound marks the chat as "known" (replies allowed) — even beyond history_days, without
        // keeping content. STOP opts out; any later live message opts back in.
        await this.store.markInbound(this.id, ev.chat_id, new Date(ev.timestamp), history ? (isOptOut(ev.text) ? true : null) : isOptOut(ev.text))
      }
      if (history && ts < cutoff) continue // older than history_days: not forwarded to ARC
      if (history) {
        batch.push(ev)
        if (batch.length === 200) await this.fwd.send({ events: batch.splice(0) })
      } else {
        await this.fwd.send({ event: ev })
      }
    }
    if (batch.length) await this.fwd.send({ events: batch })
  }

  private async cachedGroup(jid: string): Promise<GroupMetadata | undefined> {
    if (!isJidGroup(jid)) return undefined
    const c = this.groups.get(jid)
    if (c && Date.now() - c.at < GROUP_TTL) return c.meta
    if (!this.sock) return c?.meta
    try {
      const meta = await this.sock.groupMetadata(jid)
      this.groups.set(jid, { meta, at: Date.now() })
      return meta
    } catch {
      return c?.meta
    }
  }

  async listGroups(): Promise<{ jid: string; meta: GroupMeta }[]> {
    if (this.groupList && Date.now() - this.groupList.at < GROUP_LIST_TTL) return this.groupList.items
    if (!this.sock) throw new Error('sesi belum terhubung')
    const all = await this.sock.groupFetchAllParticipating()
    const items = Object.values(all).map(g => {
      this.groups.set(g.id, { meta: g, at: Date.now() })
      return { jid: g.id, meta: toGroupMeta(g.subject, g.participants) }
    })
    this.groupList = { at: Date.now(), items }
    return items
  }

  /** Profile lookup for the Identity agent: cached 7 days and rate-limited per hour. */
  async profile(jid: string): Promise<Profile> {
    const norm = jid.includes('@') ? jidNormalizedUser(jid) : jid.replace(/\D/g, '') + '@s.whatsapp.net'
    const cached = this.profiles.get(norm)
    if (cached && Date.now() - cached.at < PROFILE_TTL) return cached.p
    const c = this.contacts.get(norm) ?? {}
    const p: Profile = { name: c.verifiedName ?? c.name ?? c.notify ?? '', about: '', has_photo: false, business: c.verifiedName ?? '' }
    const hourAgo = Date.now() - 3_600_000
    this.lookups = this.lookups.filter(t => t > hourAgo)
    if (!this.sock || this.lookups.length >= this.policy.profileLookupsPerHour || isLidUser(norm)) return p
    this.lookups.push(Date.now())
    const status = await this.sock.fetchStatus(norm).catch(() => undefined)
    const s = status?.[0]?.status as { status?: string } | string | undefined
    p.about = typeof s === 'string' ? s : (s?.status ?? '')
    p.has_photo = !!(await this.sock.profilePictureUrl(norm, 'preview').catch(() => undefined))
    this.profiles.set(norm, { at: Date.now(), p })
    return p
  }

  /** Sends like a person: briefly available, "typing…", then the message. */
  async sendText(chat: string, text: string, typing: number): Promise<string> {
    const sock = this.sock
    if (!sock) throw new Error('sesi belum terhubung')
    await sock.sendPresenceUpdate('available').catch(() => {})
    await sock.sendPresenceUpdate('composing', chat).catch(() => {})
    await new Promise(r => setTimeout(r, typing))
    await sock.sendPresenceUpdate('paused', chat).catch(() => {})
    const m = await sock.sendMessage(chat, { text })
    await sock.sendPresenceUpdate('unavailable').catch(() => {})
    if (!m?.key.id) throw new Error('WhatsApp tidak mengembalikan id pesan')
    if (m.message) {
      this.recent.set(m.key.id, m.message)
      if (this.recent.size > 500) this.recent.delete(this.recent.keys().next().value as string)
    }
    return m.key.id
  }
}

/** All sessions of this bridge. */
export class Manager {
  private sessions = new Map<string, Session>()

  constructor(private store: PgStore, private fwd: Forwarder, private policy: Policy, private log: Logger) {}

  get(id: string): Session | undefined { return this.sessions.get(id) }

  statuses(): Record<string, Status> {
    return Object.fromEntries([...this.sessions].map(([id, s]) => [id, s.status]))
  }

  all(): Session[] { return [...this.sessions.values()] }

  /** Reconnects every session that was linked before a restart. */
  async restore(): Promise<void> {
    for (const meta of await this.store.sessions()) {
      if (!meta.pairedAt) continue
      const s = new Session(meta, this.store, this.fwd, this.policy, this.log.child({ session: meta.id }))
      this.sessions.set(meta.id, s)
      await s.start().catch(err => this.log.warn({ session: meta.id, err: String(err) }, 'restore failed'))
    }
  }

  /** Starts pairing (QR). An already linked session is returned as is. */
  async create(id: string, label: string, historyDays: number): Promise<Snapshot> {
    const existing = this.sessions.get(id)
    if (existing && (existing.status === 'connected' || (existing.meta.pairedAt && existing.reason === 'reconnecting'))) return existing.snapshot()
    if (existing) await existing.stop(false)
    await this.store.deleteSession(id) // fresh credentials for a new QR
    const meta: SessionMeta = { id, label, historyDays: historyDays > 0 ? historyDays : 30, pairedAt: null, phone: '' }
    await this.store.upsertSession(meta)
    const s = new Session(meta, this.store, this.fwd, this.policy, this.log.child({ session: id }))
    this.sessions.set(id, s)
    const first = s.firstState(20_000)
    await s.start()
    return first
  }

  async remove(id: string): Promise<boolean> {
    const s = this.sessions.get(id)
    if (!s) return false
    await s.stop(true)
    this.sessions.delete(id)
    await this.store.deleteSession(id)
    await this.fwd.send({ status: { session: id, status: 'disconnected', phone: '', qr: '', reason: 'unlinked' } })
    return true
  }

  async stopAll(): Promise<void> {
    for (const s of this.sessions.values()) await s.stop(false)
  }
}
