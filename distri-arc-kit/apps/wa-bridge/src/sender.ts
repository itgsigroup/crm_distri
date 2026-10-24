import type { Policy } from './config.js'
import { evaluate, sendGapMs, SHORT_TEXT, typingMs, type GuardCode } from './guard.js'
import { textHash } from './sign.js'
import type { Store } from './store.js'
import { guardText, type Content } from './content.js'

/** What the sender needs from a live WhatsApp session. */
/** The message a reply quotes (WhatsApp "Balas"): its key and text, shown above the reply on the customer's phone. */
export interface Quote { id: string; from_me: boolean; participant?: string; text: string }

export interface SendTarget {
  id: string
  connected(): boolean
  sendText(chat: string, text: string, typingMs: number, quote?: Quote, mentions?: string[]): Promise<string>
  /** A file, voice note or poll (text = caption); returns the id and the media's download reference. */
  sendContent(chat: string, content: Content, text: string, typingMs: number, quote?: Quote, mentions?: string[]): Promise<{ wamid: string; ref?: string }>
  editText(chat: string, wamid: string, text: string): Promise<void>
  deleteText(chat: string, wamid: string): Promise<void>
}

export class SendError extends Error {
  constructor(readonly status: number, readonly code: GuardCode | 'not_approved' | 'not_linked' | 'bad_request', message: string, readonly retryAfterSec?: number) {
    super(message)
  }
}

const sleep = (ms: number) => new Promise(r => setTimeout(r, ms))

/**
 * Sends one approved message. Order: idempotency by action id → human approval
 * confirmed by the API → session linked → anti-ban guard → human-like pacing.
 * Sends of one session are serialised so the random gap applies between them.
 */
export class Sender {
  private chains = new Map<string, Promise<unknown>>()
  private lastSendAt = new Map<string, number>()

  constructor(
    private store: Store,
    private approvals: { actionApproved(id: string): Promise<boolean> },
    private policy: Policy,
    private opts: { now?: () => Date; sleep?: (ms: number) => Promise<void>; rnd?: () => number } = {},
  ) {}

  private now(): Date { return this.opts.now ? this.opts.now() : new Date() }

  async send(target: SendTarget, chat: string, text: string, actionId: string, quote?: Quote, content?: Content, mentions?: string[], live = false): Promise<{ wamid: string; duplicate: boolean; ref?: string }> {
    if (!actionId) throw new SendError(403, 'not_approved', 'kirim ditolak — action belum disetujui manusia')
    if (!chat || (!text.trim() && !content)) throw new SendError(400, 'bad_request', 'chat_id dan text wajib')
    const gtext = guardText(content, text)
    const prior = await this.store.sentForAction(target.id, actionId)
    if (prior) return { wamid: prior.wamid, duplicate: true } // retried request: never send twice
    if (!(await this.approvals.actionApproved(actionId))) throw new SendError(403, 'not_approved', 'kirim ditolak — action belum disetujui manusia')
    if (!target.connected()) throw new SendError(409, 'not_linked', 'sesi belum terhubung')

    const run = async () => {
      const again = await this.store.sentForAction(target.id, actionId)
      if (again) return { wamid: again.wamid, duplicate: true }
      const facts = await this.store.facts(target.id, chat, gtext, this.now(), live)
      const verdict = evaluate(this.policy, { ...facts, shortText: gtext.trim().length < SHORT_TEXT })
      if (!verdict.ok) {
        const status = verdict.code === 'quiet_hours' ? 409 : ['opted_out', 'first_contact', 'broadcast'].includes(verdict.code) ? 403 : 429
        throw new SendError(status, verdict.code, verdict.reason, verdict.retryAfterSec)
      }
      const last = this.lastSendAt.get(target.id)
      const gap = sendGapMs(this.policy, this.opts.rnd)
      const wait = last === undefined ? 0 : Math.max(0, last + gap - this.now().getTime())
      if (wait > 0) await (this.opts.sleep ?? sleep)(wait)
      let wamid: string
      let ref: string | undefined
      if (content) ({ wamid, ref } = await target.sendContent(chat, content, text, typingMs(this.policy, text || 'x'.repeat(20)), quote, mentions))
      else wamid = await target.sendText(chat, text, typingMs(this.policy, text), quote, mentions)
      const at = this.now()
      this.lastSendAt.set(target.id, at.getTime())
      await this.store.recordSend({ session: target.id, chat, actionId, wamid, at, hash: textHash(gtext), live })
      return ref ? { wamid, duplicate: false, ref } : { wamid, duplicate: false }
    }
    return this.serial(target.id, run)
  }

  /**
   * Edits a message this number sent, for an approved edit action. Same order as send (idempotency, human approval,
   * linked); no anti-ban caps — an edit adds no message to the chat — but it waits its turn behind the number's sends.
   */
  async edit(target: SendTarget, chat: string, wamid: string, text: string, actionId: string): Promise<{ wamid: string; duplicate: boolean }> {
    if (!text.trim()) throw new SendError(400, 'bad_request', 'chat_id, wamid dan text wajib')
    return this.change(target, chat, wamid, actionId, () => target.editText(chat, wamid, text), textHash(text))
  }

  /** Deletes a message this number sent, for everyone, for an approved delete action (same checks as an edit). */
  async revoke(target: SendTarget, chat: string, wamid: string, actionId: string): Promise<{ wamid: string; duplicate: boolean }> {
    return this.change(target, chat, wamid, actionId, () => target.deleteText(chat, wamid), textHash('revoke:' + wamid))
  }

  private async change(target: SendTarget, chat: string, wamid: string, actionId: string, apply: () => Promise<void>, hash: string): Promise<{ wamid: string; duplicate: boolean }> {
    if (!actionId) throw new SendError(403, 'not_approved', 'perubahan ditolak — action belum disetujui manusia')
    if (!chat || !wamid) throw new SendError(400, 'bad_request', 'chat_id dan wamid wajib')
    const prior = await this.store.sentForAction(target.id, actionId)
    if (prior) return { wamid: prior.wamid, duplicate: true }
    if (!(await this.approvals.actionApproved(actionId))) throw new SendError(403, 'not_approved', 'edit ditolak — action belum disetujui manusia')
    if (!target.connected()) throw new SendError(409, 'not_linked', 'sesi belum terhubung')
    return this.serial(target.id, async () => {
      const again = await this.store.sentForAction(target.id, actionId)
      if (again) return { wamid: again.wamid, duplicate: true }
      await apply()
      await this.store.recordSend({ session: target.id, chat, actionId, wamid, at: this.now(), hash })
      return { wamid, duplicate: false }
    })
  }

  private serial<T>(session: string, run: () => Promise<T>): Promise<T> {
    const prev = this.chains.get(session) ?? Promise.resolve()
    const next = prev.catch(() => {}).then(run)
    this.chains.set(session, next)
    return next
  }
}
