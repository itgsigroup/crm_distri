import { isLidUser, type WACallEvent } from 'baileys'
import type { Resolver, WaEvent } from './events.js'

// WhatsApp calls (ADR 0062). The linked device hears a call's signalling — offer, accept, reject, timeout,
// terminate — but carries no audio: the phone rings and talks. The bridge never answers or rejects a call; it only
// records each one-to-one call once it ends, as a "call" bubble in the chat ("Panggilan suara tak terjawab").

export type CallOutcome = 'missed' | 'answered' | 'rejected'

interface Ringing { id: string; from: string; chatId: string; callerPn?: string; video: boolean; startedAt: Date; acceptedAt?: Date }

/** A call that ended: who called, when it started, how it ended and how long the talk was (answered only). */
export interface EndedCall { id: string; from: string; chatId: string; callerPn?: string; video: boolean; startedAt: Date; outcome: CallOutcome; seconds: number }

/** A ring ends by timeout within about a minute; a call that "terminates" later without an accept seen here was
 * picked up on another device (the phone) and talked. */
const RING_MS = 60_000
/** Calls still open after this are dropped (a lost terminate). */
const STALE_MS = 3 * 60 * 60_000

/** Follows calls from offer to their end. */
export class CallTracker {
  private calls = new Map<string, Ringing>()

  /** Takes one call update; returns the call when it has just ended, once per call. */
  update(ev: WACallEvent, now = new Date()): EndedCall | undefined {
    this.sweep(now)
    if (ev.isGroup || ev.groupJid) return undefined // group calls: not a conversation with one customer
    let c = this.calls.get(ev.id)
    if (!c) {
      c = { id: ev.id, from: ev.from, chatId: ev.chatId, callerPn: ev.callerPn, video: !!ev.isVideo, startedAt: ev.date ?? now }
      this.calls.set(ev.id, c)
    }
    if (ev.callerPn) c.callerPn = ev.callerPn
    if (ev.isVideo) c.video = true
    switch (ev.status) {
      case 'accept':
        c.acceptedAt ??= ev.date ?? now
        return undefined
      case 'reject':
      case 'timeout':
      case 'terminate': {
        this.calls.delete(ev.id)
        const end = ev.date ?? now
        let outcome: CallOutcome = 'missed'
        let seconds = 0
        if (ev.status === 'reject') outcome = 'rejected'
        else if (c.acceptedAt) {
          outcome = 'answered'
          seconds = Math.max(0, Math.round((end.getTime() - c.acceptedAt.getTime()) / 1000))
        } else if (ev.status === 'terminate' && end.getTime() - c.startedAt.getTime() > RING_MS) {
          outcome = 'answered' // talked on the phone; the length is not known here
        }
        return { id: c.id, from: c.from, chatId: c.chatId, callerPn: c.callerPn, video: c.video, startedAt: c.startedAt, outcome, seconds }
      }
    }
    return undefined
  }

  private sweep(now: Date) {
    for (const [id, c] of this.calls) if (now.getTime() - c.startedAt.getTime() > STALE_MS) this.calls.delete(id)
  }
}

/** The bubble text, as WhatsApp words it: "Panggilan suara tak terjawab", "Panggilan video · 3:12". */
export function callText(c: Pick<EndedCall, 'video' | 'outcome' | 'seconds'>): string {
  const kind = c.video ? 'Panggilan video' : 'Panggilan suara'
  if (c.outcome === 'missed') return `${kind} tak terjawab`
  if (c.outcome === 'rejected') return `${kind} ditolak`
  if (!c.seconds) return `${kind} masuk`
  return `${kind} masuk · ${Math.floor(c.seconds / 60)}:${String(c.seconds % 60).padStart(2, '0')}`
}

const digits = (jid: string | null | undefined): string => (jid ?? '').split('@')[0].split(':')[0].replace(/\D/g, '')

/** The ended call as a bridge event (wamid "call:<id>": stored once however often it is sent). A call this number
 * made itself, or one whose number cannot be resolved, is not recorded. */
export async function callEvent(session: string, c: EndedCall, r: Resolver): Promise<WaEvent | null> {
  let chat = c.callerPn && !isLidUser(c.callerPn) ? c.callerPn : c.from || c.chatId
  if (isLidUser(chat)) chat = (await r.pnForLid(chat)) ?? ''
  const from = digits(chat)
  if (!from || from === r.ownPhone) return null
  return {
    wamid: 'call:' + c.id,
    session,
    account: r.ownPhone,
    from,
    to: r.ownPhone,
    chat_id: from + '@s.whatsapp.net',
    is_group: false,
    sender_name: '',
    text: callText(c),
    media_meta: { kind: 'call', mime: '', file_name: '', size: 0, ...(c.seconds ? { seconds: c.seconds } : {}), call: c.outcome, ...(c.video ? { video: true } : {}) },
    timestamp: c.startedAt.toISOString(),
    from_me: false,
    is_history: false,
    transport: 'bridge',
  }
}
