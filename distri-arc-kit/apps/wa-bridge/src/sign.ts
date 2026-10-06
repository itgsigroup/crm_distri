import { createHmac, timingSafeEqual } from 'node:crypto'

/** Hex HMAC-SHA256 of body — the API ↔ bridge shared-secret signature (X-ARC-Signature). */
export function sign(secret: string, body: string | Buffer): string {
  return createHmac('sha256', secret).update(body).digest('hex')
}

export function verify(secret: string, body: string | Buffer, sig: string | undefined): boolean {
  if (!sig) return false
  const want = Buffer.from(sign(secret, body))
  const got = Buffer.from(sig.replace(/^sha256=/, ''))
  return want.length === got.length && timingSafeEqual(want, got)
}

/** Stable short hash used to compare message texts without storing them. */
export function textHash(text: string): string {
  return createHmac('sha256', 'arc-text').update(text.trim().replace(/\s+/g, ' ').toLowerCase()).digest('hex').slice(0, 32)
}
