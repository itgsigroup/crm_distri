import type { Policy } from './config.js'
import { evaluate, sendGapMs, typingMs, type GuardCode } from './guard.js'
import { textHash } from './sign.js'
import type { Store } from './store.js'

/** What the sender needs from a live WhatsApp session. */
/** The message a reply quotes (WhatsApp "Balas"): its key and text, shown above the reply on the customer's phone. */
export interface Quote { id: string; from_me: boolean; participant?: string; text: string }

export interface SendTarget {
  id: string
  connected(): boolean
  sendText(chat: string, text: string, typingMs: number, quote?: Quote): Promise<string>
  editText(chat: string, wamid: string, text: string): Promise<void>
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

  async send(target: SendTarget, chat: string, text: string, actionId: string, quote?: Quote): Promise<{ wamid: string; duplicate: boolean }> {
    if (!actionId) throw new SendError(403, 'not_approved', 'kirim ditolak — action belum disetujui manusia')
    if (!chat || !text.trim()) throw new SendError(400, 'bad_request', 'chat_id dan text wajib')
    const prior = await this.store.sentForAction(target.id, actionId)
    if (prior) return { wamid: prior.wamid, duplicate: true } // retried request: never send twice
    if (!(await this.approvals.actionApproved(actionId))) throw new SendError(403, 'not_approved', 'kirim ditolak — action belum disetujui manusia')
    if (!target.connected()) throw new SendError(409, 'not_linked', 'sesi belum terhubung')

    const run = async () => {
      const again = await this.store.sentForAction(target.id, actionId)
      if (again) return { wamid: again.wamid, duplicate: true }
      const verdict = evaluate(this.policy, await this.store.facts(target.id, chat, text, this.now()))
      if (!verdict.ok) {
        const status = verdict.code === 'quiet_hours' ? 409 : ['opted_out', 'first_contact', 'broadcast'].includes(verdict.code) ? 403 : 429
        throw new SendError(status, verdict.code, verdict.reason, verdict.retryAfterSec)
      }
      const last = this.lastSendAt.get(target.id)
      const gap = sendGapMs(this.policy, this.opts.rnd)
      const wait = last === undefined ? 0 : Math.max(0, last + gap - this.now().getTime())
      if (wait > 0) await (this.opts.sleep ?? sleep)(wait)
      const wamid = await target.sendText(chat, text, typingMs(this.policy, text), quote)
      const at = this.now()
      this.lastSendAt.set(target.id, at.getTime())
      await this.store.recordSend({ session: target.id, chat, actionId, wamid, at, hash: textHash(text) })
      return { wamid, duplicate: false }
    }
    return this.serial(target.id, run)
  }

  /**
   * Edits a message this number sent, for an approved edit action. Same order as send (idempotency, human approval,
   * linked); no anti-ban caps — an edit adds no message to the chat — but it waits its turn behind the number's sends.
   */
  async edit(target: SendTarget, chat: string, wamid: string, text: string, actionId: string): Promise<{ wamid: string; duplicate: boolean }> {
    if (!actionId) throw new SendError(403, 'not_approved', 'edit ditolak — action belum disetujui manusia')
    if (!chat || !wamid || !text.trim()) throw new SendError(400, 'bad_request', 'chat_id, wamid dan text wajib')
    const prior = await this.store.sentForAction(target.id, actionId)
    if (prior) return { wamid: prior.wamid, duplicate: true }
    if (!(await this.approvals.actionApproved(actionId))) throw new SendError(403, 'not_approved', 'edit ditolak — action belum disetujui manusia')
    if (!target.connected()) throw new SendError(409, 'not_linked', 'sesi belum terhubung')
    return this.serial(target.id, async () => {
      const again = await this.store.sentForAction(target.id, actionId)
      if (again) return { wamid: again.wamid, duplicate: true }
      await target.editText(chat, wamid, text)
      await this.store.recordSend({ session: target.id, chat, actionId, wamid, at: this.now(), hash: textHash(text) })
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
