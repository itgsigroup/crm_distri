import { describe, expect, it } from 'vitest'
import { defaultPolicy as P } from '../src/config.js'
import { dailyCap, evaluate, inQuietHours, isOptOut, reconnectDelayMs, sendGapMs, typingMs, wibHour, type SendFacts } from '../src/guard.js'

// 10:00 WIB on a weekday = 03:00 UTC.
const at = (wibH: number, wibM = 0, day = 5) => new Date(Date.UTC(2026, 9, day, (wibH - 7 + 24) % 24, wibM) - (wibH < 7 ? 86_400_000 : 0))
const base = (over: Partial<SendFacts> = {}): SendFacts => ({
  now: at(10), pairedAt: new Date(Date.UTC(2026, 0, 1)), sentLastHour: 0, sentToday: 0, sentToChatLastHour: 0,
  lastSentToChatAt: null, sameTextChatsLastHour: 0, knownChat: true, optedOut: false, ...over,
})

describe('anti-ban guard', () => {
  it('allows a normal reply in office hours', () => {
    expect(evaluate(P, base())).toEqual({ ok: true })
  })
  it('refuses contacts who opted out', () => {
    expect(evaluate(P, base({ optedOut: true }))).toMatchObject({ ok: false, code: 'opted_out' })
  })
  it('refuses first messages to cold contacts', () => {
    expect(evaluate(P, base({ knownChat: false }))).toMatchObject({ ok: false, code: 'first_contact' })
    expect(evaluate({ ...P, requirePriorInbound: false }, base({ knownChat: false }))).toEqual({ ok: true })
  })
  it('refuses broadcast-like identical texts', () => {
    expect(evaluate(P, base({ sameTextChatsLastHour: 2 }))).toEqual({ ok: true })
    expect(evaluate(P, base({ sameTextChatsLastHour: 3 }))).toMatchObject({ ok: false, code: 'broadcast' })
  })
  it('keeps quiet hours 21.00–07.00 WIB with a retry hint', () => {
    expect(wibHour(at(21))).toBe(21)
    expect(inQuietHours(P, at(20, 59))).toBe(false)
    expect(inQuietHours(P, at(21))).toBe(true)
    expect(inQuietHours(P, at(3))).toBe(true)
    expect(inQuietHours(P, at(7))).toBe(false)
    const r = evaluate(P, base({ now: at(22) }))
    expect(r).toMatchObject({ ok: false, code: 'quiet_hours' })
    if (!r.ok) expect(r.retryAfterSec).toBe(9 * 3600)
  })
  it('enforces hourly, daily and per-chat limits', () => {
    expect(evaluate(P, base({ sentLastHour: 20 }))).toMatchObject({ code: 'hourly_limit' })
    expect(evaluate(P, base({ sentToday: 120 }))).toMatchObject({ code: 'daily_limit' })
    expect(evaluate(P, base({ sentToChatLastHour: 6 }))).toMatchObject({ code: 'chat_limit' })
  })
  it('spaces messages to the same chat', () => {
    const now = at(10)
    expect(evaluate(P, base({ now, lastSentToChatAt: new Date(now.getTime() - 5_000) }))).toMatchObject({ code: 'chat_gap', retryAfterSec: 15 })
    expect(evaluate(P, base({ now, lastSentToChatAt: new Date(now.getTime() - 25_000) }))).toEqual({ ok: true })
  })
  it('ramps the daily cap for a freshly linked device', () => {
    const paired = at(9, 0, 5)
    expect(dailyCap(P, paired, at(10, 0, 5))).toEqual({ cap: 15, warmup: true })
    expect(dailyCap(P, paired, at(10, 0, 8)).cap).toBe(60)
    expect(dailyCap(P, paired, at(10, 0, 12))).toEqual({ cap: 120, warmup: false })
    expect(evaluate(P, base({ pairedAt: paired, now: at(11, 0, 5), sentToday: 15 }))).toMatchObject({ code: 'warmup_limit' })
    expect(dailyCap({ ...P, warmupDays: 0 }, paired, at(10, 0, 5)).cap).toBe(120)
  })
  it('paces like a person', () => {
    expect(typingMs(P, 'ok')).toBe(1500)
    expect(typingMs(P, 'x'.repeat(100))).toBe(4500)
    expect(typingMs(P, 'x'.repeat(1000))).toBe(8000)
    expect(sendGapMs(P, () => 0)).toBe(2000)
    expect(sendGapMs(P, () => 0.9999)).toBe(6000)
  })
  it('backs off reconnects exponentially up to 10 minutes', () => {
    expect(reconnectDelayMs(0, () => 0.5)).toBe(5000)
    expect(reconnectDelayMs(3, () => 0.5)).toBe(40000)
    expect(reconnectDelayMs(20, () => 0.5)).toBe(600000)
  })
  it('recognises opt-out requests only when explicit', () => {
    for (const t of ['STOP', 'stop.', 'Berhenti', 'jangan hubungi saya lagi', 'Jangan kirim lagi!']) expect(isOptOut(t), t).toBe(true)
    for (const t of ['jangan lupa kirim penawaran', 'stop kontak CCTV rusak?', 'Kapan berhenti produksi?']) expect(isOptOut(t), t).toBe(false)
  })
})
