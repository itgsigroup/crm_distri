import type { WAMessage } from 'baileys'

/**
 * Downloading the media of a message someone opens in Chat. Baileys fetches it from the host carried by the stored
 * URL; when that host cannot be reached ("fetch failed": a network error, not an HTTP answer) the same file is
 * asked from WhatsApp's default media host, and then — the file may only live on a host we cannot reach — WhatsApp
 * is asked for a fresh copy (media retry). HTTP 404/410 stay with Baileys' own re-upload.
 */

/** WhatsApp's default media host (Baileys DEF_MEDIA_HOST). */
export const MEDIA_HOST = 'mmg.whatsapp.net'

export type Download = (msg: WAMessage, opts: { host?: string }) => Promise<NodeJS.ReadableStream>
export type Reupload = (msg: WAMessage) => Promise<WAMessage>
type Warn = (obj: object, msg: string) => void

/** The network cause of a failed fetch ("ECONNRESET", "UND_ERR_CONNECT_TIMEOUT"…), or '' when it is no network error. */
export function fetchFailure(e: unknown): string {
  if (!(e instanceof Error)) return ''
  const cause = (e as Error & { cause?: { code?: string; message?: string } }).cause
  if (e instanceof TypeError && /fetch failed/i.test(e.message)) return cause?.code || cause?.message || 'fetch failed'
  const code = (e as Error & { code?: string }).code ?? cause?.code ?? ''
  return /^(ECONN|ETIMEDOUT|ENOTFOUND|EAI_AGAIN|EHOSTUNREACH|ENETUNREACH|UND_ERR)/.test(code) ? code : ''
}

/** Downloads with the fallbacks above; the error of the last try is thrown. */
export async function downloadWithFallback(msg: WAMessage, download: Download, reupload: Reupload, warn: Warn): Promise<NodeJS.ReadableStream> {
  try {
    return await download(msg, {})
  } catch (e) {
    const cause = fetchFailure(e)
    if (!cause) throw e
    warn({ cause, key: msg.key }, 'media download: host unreachable, trying the default media host')
  }
  try {
    return await download(msg, { host: MEDIA_HOST })
  } catch (e) {
    const cause = fetchFailure(e)
    if (!cause) throw e
    warn({ cause, key: msg.key }, 'media download: asking WhatsApp for a fresh copy')
  }
  return download(await reupload(msg), { host: MEDIA_HOST })
}
