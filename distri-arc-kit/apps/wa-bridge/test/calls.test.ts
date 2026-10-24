import { describe, expect, it } from 'vitest'
import type { WACallEvent } from 'baileys'
import { CallTracker, callEvent, callText } from '../src/calls.js'
import type { Resolver } from '../src/events.js'

const at = (s: number) => new Date(Date.UTC(2026, 9, 23, 7, 0, s))
const ev = (status: WACallEvent['status'], s: number, x: Partial<WACallEvent> = {}): WACallEvent =>
  ({ id: 'C1', chatId: '628111@s.whatsapp.net', from: '628111@s.whatsapp.net', date: at(s), offline: false, status, ...x })
const r: Resolver = { ownPhone: '62800', pnForLid: async (lid) => (lid === '99@lid' ? '628222@s.whatsapp.net' : null), groupMeta: async () => undefined }

// The tracker sweeps by the clock; the events here are dated 23 Oct, so it runs on their time (not today's).
const tracker = () => { const t = new CallTracker(); return { update: (e: WACallEvent) => t.update(e, e.date ?? undefined) } }

// ADR 0062: a call is recorded once, when it ends, with how it ended.
describe('CallTracker', () => {
  it('answered: the talk is timed from accept to terminate', () => {
    const t = tracker()
    expect(t.update(ev('offer', 0, { isVideo: true }))).toBeUndefined()
    expect(t.update(ev('accept', 8))).toBeUndefined()
    const c = t.update(ev('terminate', 200))
    expect(c).toMatchObject({ outcome: 'answered', seconds: 192, video: true })
    expect(t.update(ev('terminate', 201))).toMatchObject({ outcome: 'missed' }) // a repeat is a new, unknown call
  })
  it('missed: timeout, or the caller hangs up while it rings', () => {
    const t = tracker()
    t.update(ev('offer', 0))
    expect(t.update(ev('timeout', 45))).toMatchObject({ outcome: 'missed', seconds: 0 })
    t.update(ev('offer', 0, { id: 'C2' }))
    expect(t.update(ev('terminate', 12, { id: 'C2' }))).toMatchObject({ outcome: 'missed' })
  })
  it('rejected, and picked up on the phone without an accept seen here', () => {
    const t = tracker()
    t.update(ev('offer', 0))
    expect(t.update(ev('reject', 5))).toMatchObject({ outcome: 'rejected' })
    t.update(ev('offer', 0, { id: 'C3' }))
    expect(t.update(ev('terminate', 300, { id: 'C3' }))).toMatchObject({ outcome: 'answered', seconds: 0 })
  })
  it('group calls are not recorded', () => {
    const t = tracker()
    t.update(ev('offer', 0, { isGroup: true }))
    expect(t.update(ev('timeout', 40, { isGroup: true }))).toBeUndefined()
  })
})

describe('callEvent', () => {
  it('words the bubble like WhatsApp', () => {
    expect(callText({ video: false, outcome: 'missed', seconds: 0 })).toBe('Panggilan suara tak terjawab')
    expect(callText({ video: true, outcome: 'rejected', seconds: 0 })).toBe('Panggilan video ditolak')
    expect(callText({ video: false, outcome: 'answered', seconds: 192 })).toBe('Panggilan suara masuk · 3:12')
  })
  it('stores once per call id, on the caller\'s phone number (a LID resolved)', async () => {
    const e = await callEvent('s1', { id: 'C9', from: '99@lid', chatId: '99@lid', video: false, startedAt: at(0), outcome: 'missed', seconds: 0 }, r)
    expect(e).toMatchObject({ wamid: 'call:C9', chat_id: '628222@s.whatsapp.net', from: '628222', from_me: false, text: 'Panggilan suara tak terjawab', media_meta: { kind: 'call', call: 'missed' } })
    expect(await callEvent('s1', { id: 'C8', from: '62800@s.whatsapp.net', chatId: '', video: false, startedAt: at(0), outcome: 'missed', seconds: 0 }, r)).toBeNull()
    expect(await callEvent('s1', { id: 'C7', from: '77@lid', chatId: '77@lid', video: false, startedAt: at(0), outcome: 'missed', seconds: 0 }, r)).toBeNull()
  })
})
