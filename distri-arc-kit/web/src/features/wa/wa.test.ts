import { describe, expect, it } from 'vitest'
import type { WAInsight } from '../../api/types'
import { WA_COMPARE, intentLabel, itemsText, waFilter, waText, waitLabel } from './wa'

const row = (over: Partial<WAInsight>): WAInsight => ({
  thread_id: 't', kind: 'dealer', title: 'Pak Hasan · Graha', dealer_id: 'graha', dealer_name: 'PT Graha Sentosa', sales: 'Fajar', intent: 'order',
  items: [], last_text: '', inbound: 1, outbound: 0, last_in_at: null, last_out_at: null, waiting_since: null, wait_min: 0, reply_median_min: null,
  needs_reply: false, answered: false, priority: 0, signal_ids: [], proposal_ids: [], summary: '', confidence: null, ...over,
})

describe('Analisa WhatsApp helpers', () => {
  it('labels waiting times like the Go side', () => {
    expect(waitLabel(45)).toBe('45 menit')
    expect(waitLabel(300)).toBe('5 jam')
    expect(waitLabel(3 * 24 * 60)).toBe('3 hari')
  })
  it('renders items and intents', () => {
    expect(itemsText([{ name: 'Kamera IP 4MP', qty: 10 }, { name: 'HDD 2TB', qty: 2 }])).toBe('10 Kamera IP 4MP + 2 HDD 2TB')
    expect(intentLabel('release')).toBe('Minta rilis kredit')
    expect(intentLabel('none')).toBe('Tanpa pesan pelanggan')
  })
  it('sorts answered conversations below the waiting ones', () => {
    const waiting = row({ waiting_since: '2026-10-05T01:00:00Z', wait_min: 30 })
    const answered = row({ waiting_since: null, wait_min: 0 })
    expect(WA_COMPARE.menunggu(waiting, answered)).toBeGreaterThan(0)
  })
  it('filters by chip and searches the AI line', () => {
    const rows = [row({ needs_reply: true, waiting_since: 'x' }), row({ intent: 'ask_price', summary: 'minta harga NVR' })]
    expect(waFilter(rows, 'reply')).toHaveLength(1)
    expect(waFilter(rows, 'ask_price')).toHaveLength(1)
    expect(waText(rows[1])).toContain('minta harga NVR')
  })
})
