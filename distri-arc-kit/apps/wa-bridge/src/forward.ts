import { createReadStream } from 'node:fs'
import { access, appendFile, mkdir, rename, unlink } from 'node:fs/promises'
import { join } from 'node:path'
import { createInterface } from 'node:readline'
import type { Logger } from 'pino'
import { sign } from './sign.js'

/**
 * Delivers payloads to POST {API}/webhooks/wa. When the API is unreachable the
 * payload is appended to a JSONL queue and retried every 15 s, so no event is
 * lost; the API deduplicates by wamid, so retries are idempotent.
 *
 * The queue is read line by line, never whole: a backlog of history batches grew past V8's string limit (513 MB,
 * 14 Oct) and every start crashed on reading it — a reconnect of every linked phone each 27 s (ADR 0004 anti-ban).
 */

/** Lines of a JSONL file, streamed; a missing file has none. */
async function* lines(path: string): AsyncGenerator<string> {
  const rl = createInterface({ input: createReadStream(path, 'utf8'), crlfDelay: Infinity })
  try {
    for await (const l of rl) if (l) yield l
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err
  }
}

/** A history batch (up to 200 messages, several MB) takes the API longer to store than one live message. */
export const postTimeout = (bytes: number) => Math.min(180_000, 30_000 + Math.floor(bytes / 1_000_000) * 20_000)

export class Forwarder {
  lastEvent: Date | null = null
  private readonly queuePath: string
  private flushing = false
  // newest status delivered per session: a queued older status must not overwrite it after an outage
  private statusSent = new Map<string, string>()

  constructor(private apiUrl: string, private secret: string, dataDir: string, private log: Logger, private fetchImpl: typeof fetch = fetch) {
    this.queuePath = join(dataDir, 'queue.jsonl')
  }

  private async post(body: string): Promise<void> {
    const res = await this.fetchImpl(this.apiUrl + '/webhooks/wa', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-ARC-Signature': sign(this.secret, body) },
      body,
      signal: AbortSignal.timeout(postTimeout(Buffer.byteLength(body))),
    })
    if (!res.ok) throw new Error(`api HTTP ${res.status}: ${(await res.text()).slice(0, 300)}`)
  }

  /** Forwards {event}, {events} or {status}. Status snapshots carry their time (`at`). */
  async send(payload: unknown): Promise<void> {
    const p = payload as { status?: { session: string; at?: string } }
    if (p.status && !p.status.at) p.status = { ...p.status, at: new Date().toISOString() }
    const body = JSON.stringify(p)
    this.lastEvent = new Date()
    try {
      await this.post(body)
      this.noteStatus(body)
    } catch (err) {
      this.log.warn({ err: String(err) }, 'forward failed, queued')
      await this.enqueue(body)
    }
  }

  private noteStatus(body: string) {
    const st = (JSON.parse(body) as { status?: { session: string; at?: string } }).status
    if (st?.at && (this.statusSent.get(st.session) ?? '') < st.at) this.statusSent.set(st.session, st.at)
  }

  /** A queued status older than one already delivered for its session is stale. */
  private stale(body: string): boolean {
    const st = (JSON.parse(body) as { status?: { session: string; at?: string } }).status
    return !!st?.at && (this.statusSent.get(st.session) ?? '') > st.at
  }

  private async enqueue(body: string): Promise<void> {
    await mkdir(join(this.queuePath, '..'), { recursive: true })
    await appendFile(this.queuePath, body + '\n', { mode: 0o600 })
  }

  /** Payloads not delivered yet: the queue plus a batch being flushed (or left by a restart mid-flush). */
  async queueLength(): Promise<number> {
    let n = 0
    for (const path of [this.queuePath, this.queuePath + '.flushing']) for await (const _ of lines(path)) n++
    return n
  }

  /** Retries queued payloads in order; anything still failing stays queued. */
  async flush(): Promise<void> {
    if (this.flushing) return
    this.flushing = true
    const work = this.queuePath + '.flushing'
    try {
      // a batch left by a restart mid-flush goes first; renaming the queue onto it would have overwritten it (lost
      // the rest of a freshly linked number's history, 12 Oct). The queue waits for the next round.
      const leftover = await access(work).then(() => true, () => false)
      if (!leftover) {
        try {
          await rename(this.queuePath, work)
        } catch {
          return
        }
      }
      let n = 0
      let failed = false
      for await (const l of lines(work)) {
        // after a failure the rest goes back to the queue, in order, for the next round
        if (failed) {
          await this.enqueue(l)
          continue
        }
        n++
        if (this.stale(l)) continue
        try {
          await this.post(l)
          this.noteStatus(l)
        } catch {
          failed = true
          await this.enqueue(l)
        }
      }
      if (!failed) this.log.info({ n, leftover }, 'queue flushed')
      await unlink(work).catch(() => {})
    } finally {
      this.flushing = false
    }
  }

  start(): NodeJS.Timeout {
    return setInterval(() => void this.flush(), 15_000)
  }

  /** Asks the API whether actionId was approved by a human. */
  async actionApproved(actionId: string): Promise<boolean> {
    const res = await this.fetchImpl(this.apiUrl + '/bridge/actions/' + encodeURIComponent(actionId), {
      headers: { 'X-ARC-Signature': sign(this.secret, actionId) },
      signal: AbortSignal.timeout(10_000),
    })
    if (res.status !== 200) return false
    const out = (await res.json()) as { approved?: boolean }
    return out.approved === true
  }
}
