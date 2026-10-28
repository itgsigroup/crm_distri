import makeWASocket, {
  Browsers, DisconnectReason, downloadMediaMessage, fetchLatestBaileysVersion, isJidGroup, isLidUser, jidNormalizedUser, makeCacheableSignalKeyStore,
  type Contact, type GroupMetadata, type WAMessageKey, type WASocket, type proto,
} from 'baileys'
import type { Logger } from 'pino'
import type { Policy } from './config.js'
import { CallTracker, callEvent } from './calls.js'
import { ffmpegToOgg, toMessage, type AudioConverter, type Content } from './content.js'
import { decodeMediaRef, isConversation, mediaRef, toGroupMeta, toWaEvent, type GroupMeta, type Resolver, type WaEvent } from './events.js'
import type { Forwarder } from './forward.js'
import { downloadWithFallback } from './media.js'
import { isOptOut, reconnectDelayMs } from './guard.js'
import type { PgStore } from './pgstore.js'
import type { Quote, SendTarget } from './sender.js'
import type { SessionMeta } from './store.js'

export type Status = 'pairing' | 'connected' | 'disconnected'

export interface Snapshot { session: string; status: Status; phone: string; qr: string; reason?: string }

export interface Profile { name: string; about: string; has_photo: boolean; business: string }

const GROUP_TTL = 60 * 60_000
const GROUP_LIST_TTL = 10 * 60_000
const PROFILE_TTL = 7 * 24 * 60 * 60_000
const STABLE_MS = 5 * 60_000
// On-demand history (ADR 0028): one request per number every HISTORY_GAP_MS, at most HISTORY_PER_HOUR an hour —
// WhatsApp Web asks the same way while someone scrolls up a chat.
const HISTORY_GAP_MS = 4_000
const HISTORY_PER_HOUR = 400
// Close codes after which reconnecting would only look abusive: stop and ask a human.
const FATAL = new Set<number>([DisconnectReason.loggedOut, DisconnectReason.forbidden, DisconnectReason.connectionReplaced, DisconnectReason.multideviceMismatch, DisconnectReason.badSession])

const digits = (jid?: string | null) => (jid ?? '').split('@')[0].split(':')[0].replace(/\D/g, '')
const sleep = (ms: number) => new Promise(r => setTimeout(r, ms))

/** Media files a person may open at once per number: WhatsApp Web downloads a handful in parallel, not hundreds. */
const MEDIA_PARALLEL = 4

/** The number cannot fetch media right now (not connected, or too many downloads at once). */
export class MediaBusy extends Error {
  constructor(msg: string, public status: 409 | 429) { super(msg) }
}

/** The number cannot ask its phone for history right now (not connected, or the request pace is used up). */
export class HistoryBusy extends Error {
  constructor(msg: string, public status: 409 | 429) { super(msg) }
}

/** Whether another on-demand history request fits the pace: the gap since the last one and the hourly cap. */
export function historyAllowed(sent: number[], now: number): boolean {
  const recent = sent.filter(t => t > now - 3_600_000)
  return recent.length < HISTORY_PER_HOUR && (recent.length === 0 || now - recent[recent.length - 1] >= HISTORY_GAP_MS)
}

/** "ABCD1234" → "ABCD-1234", as WhatsApp shows it. */
// History goes to the API in batches small enough to be stored well within the post timeout while several numbers
// sync at once (200 timed out on 14 Oct, was retried and stored again and again).
export const HISTORY_BATCH = 50

export const formatPairCode = (c: string) => (c.length === 8 ? `${c.slice(0, 4)}-${c.slice(4)}` : c)

/** How the phone knows a contact: saved name, verified business name, then their own push name. */
export function contactName(c: Partial<Contact> | undefined): string {
  return (c?.name || c?.verifiedName || c?.notify || '').trim()
}

/**
 * One linked WhatsApp number. Connection hygiene to stay under WhatsApp's radar:
 * a stable device name, not marked online on connect (the phone keeps its
 * notifications), history kept for historyDays (a year), cached group metadata,
 * rate-limited profile lookups and history requests, exponential reconnect backoff, and no reconnect after a
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
  private photoLookups: number[] = []
  private historyAsks: number[] = []
  private downloads = 0
  private contacts = new Map<string, Partial<Contact>>()
  private recent = new Map<string, proto.IMessage>() // for retry receipts (getMessage)
  private unread = new Map<string, WAMessageKey[]>() // inbound not yet read, per chat: read before replying
  private callTracker = new CallTracker() // calls from offer to their end (ADR 0062)
  private waiters: ((s: Snapshot) => void)[] = []
  /** Link with a code typed on the phone instead of a QR scan. */
  codeMode = false
  codePhone = '' // the number the code is requested for
  private codeRequested = false

  constructor(
    public meta: SessionMeta,
    private store: PgStore,
    private fwd: Forwarder,
    private policy: Policy,
    private log: Logger,
    private deviceName = 'Distri ARC',
    private minHistoryDays = 365,
  ) {}

  get id(): string { return this.meta.id }

  /** Days of history forwarded: the session's own setting, never less than the bridge's floor. */
  historyDays(): number { return Math.max(this.meta.historyDays, this.minHistoryDays) }

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
      browser: Browsers.ubuntu(this.deviceName),
      logger: this.log,
      markOnlineOnConnect: false,
      // ask the phone for its full history on linking (the bridge forwards only historyDays of it), and accept every
      // history sync type — the default drops FULL, and ON_DEMAND carries the backfill requests
      syncFullHistory: true,
      shouldSyncHistoryMessage: () => true,
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
    sock.ev.on('messaging-history.set', ({ messages, contacts }) => {
      for (const c of contacts ?? []) this.contacts.set(c.id, { ...this.contacts.get(c.id), ...c })
      void this.forwardMessages(messages, true)
    })
    sock.ev.on('contacts.upsert', cs => { for (const c of cs) this.contacts.set(c.id, { ...this.contacts.get(c.id), ...c }) })
    sock.ev.on('contacts.update', cs => { for (const c of cs) if (c.id) this.contacts.set(c.id, { ...this.contacts.get(c.id), ...c }) })
    sock.ev.on('groups.update', gs => { for (const g of gs) if (g.id) this.groups.delete(g.id); this.groupList = undefined })
    sock.ev.on('group-participants.update', g => { this.groups.delete(g.id); this.groupList = undefined })
    sock.ev.on('call', calls => { for (const c of calls) void this.forwardCall(c) })
  }

  /** A one-to-one call that just ended goes to Distri ARC as a call bubble; the call itself is left to the phone
   * (never answered or rejected here). A call is inbound contact: the chat counts as known, an opt-out stays. */
  private async forwardCall(update: import('baileys').WACallEvent) {
    const ended = this.callTracker.update(update)
    if (!ended) return
    try {
      const ev = await callEvent(this.id, ended, this.resolver())
      if (!ev) return
      const name = contactName(this.contacts.get(ev.chat_id))
      if (name) ev.chat_name = name
      await this.store.markInbound(this.id, ev.chat_id, new Date(ev.timestamp), null) // known chat; a STOP stays
      await this.fwd.send({ event: ev })
    } catch (err) {
      this.log.warn({ session: this.id, err: String(err) }, 'forward call failed')
    }
  }

  private async onConnection(u: Partial<import('baileys').ConnectionState>, wasRegistered: boolean) {
    if (u.qr && this.codeMode) {
      // The socket is ready to link: ask WhatsApp for a code once; later QR rotations are ignored (the code stays valid).
      if (!this.codeRequested && this.sock) {
        this.codeRequested = true
        try {
          const code = await this.sock.requestPairingCode(this.codePhone || this.meta.id)
          this.qr = 'code:' + formatPairCode(code)
          this.setStatus('pairing')
        } catch (err) {
          this.log.warn({ session: this.id, err: String(err) }, 'pairing code request failed')
          this.codeRequested = false
        }
      }
    } else if (u.qr) {
      this.qr = u.qr
      this.setStatus('pairing')
    }
    if (u.isNewLogin) {
      // QR scanned / code typed: the phone is logging in; the UI shows a loader until "open" (Distri ARC: LinkingPrefix)
      this.qr = 'linking:'
      this.setStatus('pairing')
    }
    if (u.connection === 'open') {
      this.codeMode = false
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

  /** What the phone calls a member: its saved name, WhatsApp name or push name ('' when unknown). */
  private contactName(jid: string, phone: string): string {
    const c = this.contacts.get(jid) ?? (phone ? this.contacts.get(phone + '@s.whatsapp.net') : undefined)
    return c?.name || c?.verifiedName || c?.notify || ''
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
    const cutoff = Date.now() / 1000 - this.historyDays() * 86_400
    const r = this.resolver()
    const batch: WaEvent[] = []
    for (const m of messages) {
      const ts = Number(m.messageTimestamp ?? 0)
      const ev = await toWaEvent(this.id, m, r, history).catch(err => { this.log.warn({ err: String(err) }, 'map message failed'); return null })
      if (!ev) continue
      if (!ev.is_group) {
        const name = contactName(this.contacts.get(ev.chat_id)) || contactName(this.contacts.get(m.key.remoteJid ?? ''))
        if (name) ev.chat_name = name
      }
      if (!history) this.trackUnread(ev.chat_id, m.key, ev.from_me)
      if (!ev.from_me) {
        // Inbound marks the chat as "known" (replies allowed) — even beyond history_days, without
        // keeping content. STOP opts out; any later live message opts back in.
        await this.store.markInbound(this.id, ev.chat_id, new Date(ev.timestamp), history ? (isOptOut(ev.text) ? true : null) : isOptOut(ev.text))
      }
      if (history && ts < cutoff) continue // older than history_days: not forwarded to Distri ARC
      if (history) {
        batch.push(ev)
        if (batch.length === HISTORY_BATCH) await this.fwd.send({ events: batch.splice(0) })
      } else {
        await this.fwd.send({ event: ev })
      }
    }
    if (batch.length) await this.fwd.send({ events: batch })
  }

  /** Remembers live inbound keys per chat; a message sent from the phone means the person read the chat there. */
  private trackUnread(chat: string, key: WAMessageKey, fromMe: boolean) {
    if (fromMe) {
      this.unread.delete(chat)
      return
    }
    const keys = this.unread.get(chat) ?? []
    keys.push(key)
    this.unread.set(chat, keys.slice(-20))
    if (this.unread.size > 2000) this.unread.delete(this.unread.keys().next().value as string)
  }

  /**
   * A person opened the chat in GSI Orbit: its unread inbound messages are read on WhatsApp now (blue ticks), like
   * WhatsApp Web — not only just before a reply goes out (ADR 0069). Returns how many were read.
   */
  async markRead(chat: string): Promise<number> {
    const sock = this.sock
    if (!sock || this.status !== 'connected') throw new HistoryBusy('sesi belum terhubung', 409)
    const keys = this.unread.get(chat)
    if (!keys?.length) return 0
    this.unread.delete(chat)
    await sock.readMessages(keys)
    return keys.length
  }

  /**
   * Asks the linked phone for up to `count` messages of a chat before the given one (on-demand history sync). The
   * answer arrives later through messaging-history.set and is forwarded like any history. A one-to-one chat the
   * phone addresses by LID is asked by its LID.
   */
  async fetchHistory(chat: string, oldest: { id: string; fromMe: boolean; ts: number }, count: number): Promise<void> {
    const sock = this.sock
    if (!sock || this.status !== 'connected') throw new HistoryBusy('sesi belum terhubung', 409)
    const now = Date.now()
    if (!historyAllowed(this.historyAsks, now)) throw new HistoryBusy('permintaan riwayat terlalu cepat', 429)
    this.historyAsks = [...this.historyAsks.filter(t => t > now - 3_600_000), now]
    let jid = chat
    if (!isJidGroup(chat) && !isLidUser(chat)) {
      const lid = await sock.signalRepository.lidMapping.getLIDForPN(chat).catch(() => null)
      if (lid) jid = lid
    }
    await sock.fetchMessageHistory(Math.min(Math.max(count, 1), 50), { remoteJid: jid, fromMe: oldest.fromMe, id: oldest.id }, oldest.ts)
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
    const sock = this.sock
    if (!sock) {
      if (this.groupList) return this.groupList.items // reconnecting: the last list is better than none
      throw new Error('sesi belum terhubung')
    }
    let all: Awaited<ReturnType<typeof sock.groupFetchAllParticipating>>
    try {
      all = await sock.groupFetchAllParticipating()
    } catch (err) {
      if (this.groupList) return this.groupList.items // WhatsApp refused (rate-overlimit): keep the last list
      throw err
    }
    const items = []
    for (const g of Object.values(all)) {
      this.groups.set(g.id, { meta: g, at: Date.now() })
      // a member of a LID-addressed group without its phone number: the number from the LID mapping, so the
      // profile's "grup bersama" can find the contact by number
      const participants = await Promise.all(g.participants.map(async p => {
        if (p.phoneNumber || !isLidUser(p.id)) return p
        const pn = await sock.signalRepository.lidMapping.getPNForLID(p.id).catch(() => null)
        return pn ? { ...p, phoneNumber: pn } : p
      }))
      items.push({ jid: g.id, meta: toGroupMeta(g.subject, participants, (jid, phone) => this.contactName(jid, phone)) })
    }
    this.groupList = { at: Date.now(), items }
    return items
  }

  /**
   * Saves a number as a contact of this WhatsApp account (app-state "contact" action, as WhatsApp itself does when a
   * contact is saved in the app): it syncs to the phone and, with saveOnPrimaryAddressbook, into its address book.
   */
  async saveContact(phone: string, fullName: string): Promise<void> {
    if (!this.sock) throw new Error('sesi belum terhubung')
    const jid = phone + '@s.whatsapp.net'
    const firstName = fullName.split(' ')[0]
    await this.sock.addOrEditContact(jid, { fullName, firstName, pnJid: jid, saveOnPrimaryAddressbook: true })
    const c = this.contacts.get(jid) ?? { id: jid }
    this.contacts.set(jid, { ...c, name: fullName })
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

  /**
   * A contact's or group's WhatsApp profile photo (preview: the small one for the chat list; image: the full one for
   * the profile sheet). Rate-limited per hour; the API caches the result, so a chat list does not ask again.
   * Undefined: no photo, or hidden by the contact's privacy settings.
   */
  async photo(jid: string, size: 'preview' | 'image'): Promise<{ data: Buffer; mime: string } | undefined> {
    const sock = this.sock
    if (!sock || this.status !== 'connected') throw new MediaBusy('sesi belum terhubung', 409)
    const hourAgo = Date.now() - 3_600_000
    this.photoLookups = this.photoLookups.filter(t => t > hourAgo)
    if (this.photoLookups.length >= this.policy.photoLookupsPerHour) throw new MediaBusy('batas foto profil per jam tercapai', 429)
    this.photoLookups.push(Date.now())
    const norm = jid.includes('@') ? jidNormalizedUser(jid) : jid.replace(/\D/g, '') + '@s.whatsapp.net'
    const url = await sock.profilePictureUrl(norm, size).catch(() => undefined)
    if (!url) return undefined
    const r = await fetch(url, { signal: AbortSignal.timeout(15_000) })
    if (!r.ok) return undefined
    const data = Buffer.from(await r.arrayBuffer())
    if (data.length > 2 << 20) return undefined
    return { data, mime: (r.headers.get('content-type') ?? 'image/jpeg').split(';')[0] }
  }

  /**
   * Downloads one media file a person opened in Chat, from a reference stored with the message (events.ts mediaRef).
   * Streamed through, never stored. A file expired on WhatsApp's CDN is asked from the phone again (re-upload).
   */
  async downloadMedia(ref: string): Promise<NodeJS.ReadableStream> {
    const sock = this.sock
    if (!sock || this.status !== 'connected') throw new MediaBusy('sesi belum terhubung', 409)
    if (this.downloads >= MEDIA_PARALLEL) throw new MediaBusy('terlalu banyak media dibuka bersamaan', 429)
    this.downloads++
    try {
      const ctx = { logger: this.log, reuploadRequest: sock.updateMediaMessage }
      return await downloadWithFallback(decodeMediaRef(ref), (m, opts) => downloadMediaMessage(m, 'stream', opts, ctx) as Promise<NodeJS.ReadableStream>,
        sock.updateMediaMessage, (o, msg) => this.log.warn(o, msg))
    } finally {
      this.downloads--
    }
  }

  /** Edits a text this number sent (WhatsApp allows it for 15 minutes); the chat shows it as "Diedit". */
  async editText(chat: string, wamid: string, text: string): Promise<void> {
    const sock = this.sock
    if (!sock) throw new Error('sesi belum terhubung')
    await sock.sendMessage(chat, { text, edit: { remoteJid: chat, fromMe: true, id: wamid } })
  }

  /** Deletes a message this number sent, for everyone (WhatsApp: "Pesan ini dihapus"). */
  async deleteText(chat: string, wamid: string): Promise<void> {
    const sock = this.sock
    if (!sock) throw new Error('sesi belum terhubung')
    await sock.sendMessage(chat, { delete: { remoteJid: chat, fromMe: true, id: wamid } })
  }

  /** How voice notes recorded in the browser become ogg/opus (ffmpeg; tests replace it). */
  convertAudio: AudioConverter = ffmpegToOgg

  /** A file, voice note or poll, sent like a text (presence, read, typing — "recording…" for a voice note). */
  async sendContent(chat: string, content: Content, text: string, typing: number, quote?: Quote, mentions?: string[]): Promise<{ wamid: string; ref?: string }> {
    const msg = await toMessage(content, text, this.convertAudio)
    if (mentions?.length && content.kind !== 'poll' && content.kind !== 'audio') Object.assign(msg, { mentions }) // tagged in the caption
    const m = await this.deliver(chat, msg, typing, quote, content.kind === 'audio' && content.ptt ? 'recording' : 'composing')
    return { wamid: m.key.id as string, ref: mediaRef(m) ?? undefined }
  }

  /** Sends like a person: online, reads the chat (blue ticks), pauses, "typing…", then the message. */
  async sendText(chat: string, text: string, typing: number, quote?: Quote, mentions?: string[]): Promise<string> {
    const m = await this.deliver(chat, mentions?.length ? { text, mentions } : { text }, typing, quote, 'composing')
    return m.key.id as string
  }

  private async deliver(chat: string, content: import('baileys').AnyMessageContent, typing: number, quote: Quote | undefined, presence: 'composing' | 'recording'): Promise<import('baileys').WAMessage> {
    const sock = this.sock
    if (!sock) throw new Error('sesi belum terhubung')
    await sock.sendPresenceUpdate('available').catch(() => {})
    const keys = this.unread.get(chat)
    if (keys?.length) {
      this.unread.delete(chat)
      await sock.readMessages(keys).catch(() => {})
      await sleep(1_200 + Math.floor(Math.random() * 1_800)) // reading before typing
    }
    await sock.sendPresenceUpdate(presence, chat).catch(() => {})
    await new Promise(r => setTimeout(r, typing))
    await sock.sendPresenceUpdate('paused', chat).catch(() => {})
    const quoted = quote?.id
      ? { key: { remoteJid: chat, fromMe: quote.from_me, id: quote.id, participant: quote.participant || undefined }, message: { conversation: quote.text } }
      : undefined
    const m = await sock.sendMessage(chat, content, quoted ? { quoted } : undefined)
    await sock.sendPresenceUpdate('unavailable').catch(() => {})
    if (!m?.key.id) throw new Error('WhatsApp tidak mengembalikan id pesan')
    if (m.message) {
      this.recent.set(m.key.id, m.message)
      if (this.recent.size > 500) this.recent.delete(this.recent.keys().next().value as string)
    }
    return m
  }
}

/** All sessions of this bridge. */
export class Manager {
  private sessions = new Map<string, Session>()

  constructor(private store: PgStore, private fwd: Forwarder, private policy: Policy, private log: Logger, private deviceName = 'Distri ARC', private minHistoryDays = 365) {}

  get(id: string): Session | undefined { return this.sessions.get(id) }

  statuses(): Record<string, Status> {
    return Object.fromEntries([...this.sessions].map(([id, s]) => [id, s.status]))
  }

  all(): Session[] { return [...this.sessions.values()] }

  /** Reconnects every session that was linked before a restart. */
  async restore(): Promise<void> {
    for (const meta of await this.store.sessions()) {
      if (!meta.pairedAt) continue
      const s = new Session(meta, this.store, this.fwd, this.policy, this.log.child({ session: meta.id }), this.deviceName, this.minHistoryDays)
      this.sessions.set(meta.id, s)
      await s.start().catch(err => this.log.warn({ session: meta.id, err: String(err) }, 'restore failed'))
    }
  }

  /** Starts pairing (QR). An already linked session is returned as is. */
  async create(id: string, label: string, historyDays: number, phoneCode = false, phone = ''): Promise<Snapshot> {
    const existing = this.sessions.get(id)
    if (existing && (existing.status === 'connected' || (existing.meta.pairedAt && existing.reason === 'reconnecting'))) return existing.snapshot()
    if (existing) await existing.stop(false)
    await this.store.deleteSession(id) // fresh credentials for a new QR
    const meta: SessionMeta = { id, label, historyDays: historyDays > 0 ? historyDays : this.minHistoryDays, pairedAt: null, phone: '' }
    await this.store.upsertSession(meta)
    const s = new Session(meta, this.store, this.fwd, this.policy, this.log.child({ session: id }), this.deviceName, this.minHistoryDays)
    s.codeMode = phoneCode
    s.codePhone = phone.replace(/\D/g, '')
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
