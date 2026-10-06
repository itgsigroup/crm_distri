import type { Policy } from './config.js'
import { evaluate, sendGapMs, typingMs, type GuardCode } from './guard.js'
import { textHash } from './sign.js'
import type { Store } from './store.js'

/** What the sender needs from a live WhatsApp session. */
export interface SendTarget {
  id: string
  connected(): boolean
  sendText(chat: string, text: string, typingMs: number): Promise<string>
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

  async send(target: SendTarget, chat: string, text: string, actionId: string): Promise<{ wamid: string; duplicate: boolean }> {
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
      const wamid = await target.sendText(chat, text, typingMs(this.policy, text))
      const at = this.now()
      this.lastSendAt.set(target.id, at.getTime())
      await this.store.recordSend({ session: target.id, chat, actionId, wamid, at, hash: textHash(text) })
      return { wamid, duplicate: false }
    }
    const prev = this.chains.get(target.id) ?? Promise.resolve()
    const next = prev.catch(() => {}).then(run)
    this.chains.set(target.id, next)
    return next
  }
}
