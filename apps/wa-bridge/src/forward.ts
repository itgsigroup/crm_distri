import { appendFile, mkdir, readFile, rename, unlink } from 'node:fs/promises'
import { join } from 'node:path'
import type { Logger } from 'pino'
import { sign } from './sign.js'

/**
 * Delivers payloads to POST {API}/webhooks/wa. When the API is unreachable the
 * payload is appended to a JSONL queue and retried every 15 s, so no event is
 * lost; the API deduplicates by wamid, so retries are idempotent.
 */
export class Forwarder {
  lastEvent: Date | null = null
  private readonly queuePath: string
  private flushing = false

  constructor(private apiUrl: string, private secret: string, dataDir: string, private log: Logger, private fetchImpl: typeof fetch = fetch) {
    this.queuePath = join(dataDir, 'queue.jsonl')
  }

  private async post(body: string): Promise<void> {
    const res = await this.fetchImpl(this.apiUrl + '/webhooks/wa', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-ARC-Signature': sign(this.secret, body) },
      body,
      signal: AbortSignal.timeout(30_000),
    })
    if (!res.ok) throw new Error(`api HTTP ${res.status}: ${(await res.text()).slice(0, 300)}`)
  }

  /** Forwards {event}, {events} or {status}. */
  async send(payload: unknown): Promise<void> {
    const body = JSON.stringify(payload)
    this.lastEvent = new Date()
    try {
      await this.post(body)
    } catch (err) {
      this.log.warn({ err: String(err) }, 'forward failed, queued')
      await this.enqueue(body)
    }
  }

  private async enqueue(body: string): Promise<void> {
    await mkdir(join(this.queuePath, '..'), { recursive: true })
    await appendFile(this.queuePath, body + '\n', { mode: 0o600 })
  }

  async queueLength(): Promise<number> {
    try {
      return (await readFile(this.queuePath, 'utf8')).split('\n').filter(Boolean).length
    } catch {
      return 0
    }
  }

  /** Retries queued payloads in order; anything still failing stays queued. */
  async flush(): Promise<void> {
    if (this.flushing) return
    this.flushing = true
    const work = this.queuePath + '.flushing'
    try {
      try {
        await rename(this.queuePath, work)
      } catch {
        return
      }
      const lines = (await readFile(work, 'utf8')).split('\n').filter(Boolean)
      let failedAt = -1
      for (let i = 0; i < lines.length; i++) {
        try {
          await this.post(lines[i])
        } catch {
          failedAt = i
          break
        }
      }
      if (failedAt >= 0) for (const l of lines.slice(failedAt)) await this.enqueue(l)
      else this.log.info({ n: lines.length }, 'queue flushed')
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
