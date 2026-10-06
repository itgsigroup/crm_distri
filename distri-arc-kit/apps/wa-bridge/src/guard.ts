import type { Policy } from './config.js'

// Anti-ban guard. WhatsApp flags linked devices that behave like bulk senders:
// bursts, many messages to people who never wrote first, identical texts to many
// chats, activity at night, and brand-new devices that suddenly send a lot.
// Every outbound send — already approved by a human in ARC — must also pass here.

/** Facts about the session and chat at the moment of sending (computed by the store). */
export interface SendFacts {
  now: Date
  pairedAt: Date | null          // first successful link of this device
  sentLastHour: number           // this session, sliding hour
  sentToday: number              // this session, since 00:00 WIB
  sentToChatLastHour: number
  lastSentToChatAt: Date | null
  sameTextChatsLastHour: number  // other chats that received the same text in the last hour
  knownChat: boolean             // the chat wrote to this number before (or is a group we read)
  optedOut: boolean              // the contact asked us to stop
}

export type GuardCode =
  | 'quiet_hours' | 'hourly_limit' | 'daily_limit' | 'warmup_limit' | 'chat_limit'
  | 'chat_gap' | 'first_contact' | 'broadcast' | 'opted_out'

export type GuardResult = { ok: true } | { ok: false; code: GuardCode; reason: string; retryAfterSec?: number }

const WIB_OFFSET_MIN = 7 * 60

/** Hour of day in Asia/Jakarta (UTC+7, no DST). */
export function wibHour(d: Date): number {
  return Math.floor(((d.getUTCHours() * 60 + d.getUTCMinutes() + WIB_OFFSET_MIN) % 1440) / 60)
}

/** Start of the current day in WIB, as a UTC instant. */
export function startOfWibDay(d: Date): Date {
  const local = new Date(d.getTime() + WIB_OFFSET_MIN * 60_000)
  local.setUTCHours(0, 0, 0, 0)
  return new Date(local.getTime() - WIB_OFFSET_MIN * 60_000)
}

export function inQuietHours(p: Policy, d: Date): boolean {
  if (p.quietStartHour === p.quietEndHour) return false
  const h = wibHour(d)
  return p.quietStartHour > p.quietEndHour
    ? h >= p.quietStartHour || h < p.quietEndHour
    : h >= p.quietStartHour && h < p.quietEndHour
}

/** Daily cap including the warm-up ramp after pairing a new device. */
export function dailyCap(p: Policy, pairedAt: Date | null, now: Date): { cap: number; warmup: boolean } {
  if (!p.warmupDays || !pairedAt) return { cap: p.maxPerDay, warmup: false }
  const day = Math.floor((startOfWibDay(now).getTime() - startOfWibDay(pairedAt).getTime()) / 86_400_000)
  if (day >= p.warmupDays) return { cap: p.maxPerDay, warmup: false }
  const start = Math.min(p.warmupStartPerDay, p.maxPerDay)
  const cap = Math.round(start + ((p.maxPerDay - start) * Math.max(day, 0)) / p.warmupDays)
  return { cap, warmup: true }
}

function secondsUntilQuietEnds(p: Policy, now: Date): number {
  const h = wibHour(now)
  let hours = (p.quietEndHour - h + 24) % 24
  if (hours === 0) hours = 24
  const minutes = now.getUTCMinutes()
  return Math.max(60, hours * 3600 - minutes * 60)
}

/** Decides whether an approved message may be sent now. Order: hard refusals first, then pacing. */
export function evaluate(p: Policy, f: SendFacts): GuardResult {
  if (f.optedOut) {
    return { ok: false, code: 'opted_out', reason: 'Kontak meminta berhenti dihubungi (opt-out) — kirim ditolak' }
  }
  if (p.requirePriorInbound && !f.knownChat) {
    return { ok: false, code: 'first_contact', reason: 'Kontak belum pernah mengirim pesan ke nomor ini — pesan pertama ke kontak dingin diblokir untuk mengurangi risiko blokir; hubungi lewat email/telepon atau minta kontak menyapa dulu' }
  }
  if (f.sameTextChatsLastHour >= p.maxSameTextChats) {
    return { ok: false, code: 'broadcast', reason: `Teks yang sama sudah dikirim ke ${f.sameTextChatsLastHour} chat dalam 1 jam — pola broadcast diblokir; personalisasi pesannya` }
  }
  if (inQuietHours(p, f.now)) {
    return { ok: false, code: 'quiet_hours', reason: `Jam tenang ${p.quietStartHour}.00–${p.quietEndHour}.00 WIB — kirim besok pagi`, retryAfterSec: secondsUntilQuietEnds(p, f.now) }
  }
  const { cap, warmup } = dailyCap(p, f.pairedAt, f.now)
  if (f.sentToday >= cap) {
    return warmup
      ? { ok: false, code: 'warmup_limit', reason: `Nomor baru tertaut sedang masa pemanasan — batas hari ini ${cap} pesan`, retryAfterSec: 3600 }
      : { ok: false, code: 'daily_limit', reason: `Batas ${cap} pesan/hari untuk nomor ini tercapai`, retryAfterSec: 3600 }
  }
  if (f.sentLastHour >= p.maxPerHour) {
    return { ok: false, code: 'hourly_limit', reason: `Batas ${p.maxPerHour} pesan/jam untuk nomor ini tercapai`, retryAfterSec: 600 }
  }
  if (f.sentToChatLastHour >= p.maxPerChatPerHour) {
    return { ok: false, code: 'chat_limit', reason: `Sudah ${f.sentToChatLastHour} pesan ke chat ini dalam 1 jam — tunggu balasan dulu`, retryAfterSec: 900 }
  }
  if (f.lastSentToChatAt && f.now.getTime() - f.lastSentToChatAt.getTime() < p.minGapSameChatMs) {
    const wait = Math.ceil((p.minGapSameChatMs - (f.now.getTime() - f.lastSentToChatAt.getTime())) / 1000)
    return { ok: false, code: 'chat_gap', reason: `Terlalu cepat setelah pesan sebelumnya ke chat ini — coba lagi ${wait} detik lagi`, retryAfterSec: wait }
  }
  return { ok: true }
}

/** "typing…" duration proportional to message length, clamped. */
export function typingMs(p: Policy, text: string): number {
  return Math.min(p.typingMaxMs, Math.max(p.typingMinMs, Math.round(text.length * p.typingMsPerChar)))
}

/** Random pause between consecutive sends of one session. */
export function sendGapMs(p: Policy, rnd: () => number = Math.random): number {
  return p.minDelayMs + Math.floor(rnd() * (p.maxDelayMs - p.minDelayMs + 1))
}

/** Exponential reconnect backoff with jitter: 5 s, 10 s, 20 s … capped at 10 min. */
export function reconnectDelayMs(attempt: number, rnd: () => number = Math.random): number {
  const base = Math.min(600_000, 5_000 * 2 ** Math.max(0, attempt))
  return Math.round(base * (0.8 + rnd() * 0.4))
}

const STOP_RE = /^\s*(stop|berhenti|unsubscribe|jangan\s+(hubungi|kirim|chat)(\s+saya)?\s*(lagi)?)\s*[.!]*\s*$/i

/** Whether an inbound text is an opt-out request ("STOP", "berhenti", "jangan hubungi saya lagi"). */
export function isOptOut(text: string): boolean {
  return STOP_RE.test(text)
}
