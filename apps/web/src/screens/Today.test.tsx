import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import type { Action, Today } from '../api/types'
import { UIProvider } from '../state/UIProvider'
import { TodayScreen } from './Today'

const action: Action = {
  id: 'a1', agent: 'Follow-up agent', type: 'send_email', kind: 'send',
  title: 'Kirim revisi penawaran', button_label: 'Kirim', icon: 'i-send',
  account_name: 'Akun Uji', due_label: 'Hari ini', summary: 'Ringkasan', why: '', prep: '',
  preview: 'Draf pesan', preview_from: 'Kepada: uji', context_note: 'Catatan', impact: [], steps: [],
  options: [], tags: [{ k: 'accent', t: 'Follow-up agent' }], confidence: 0.9, model: 'm',
  provenance_line: '', status: 'proposed', result_text: 'Terkirim', in_queue: true,
}

const today: Today = {
  greeting: { title: 'Selamat pagi, Uji.', sub: 'Satu hal menunggu.' },
  strip: [{ n: '1', tone: 'accent', label: 'Keputusan', scroll: 'queue-card' }],
  brief: null,
  queue: { meta: '1 item', filters: [{ key: 'all', label: 'Semua' }], items: [action] },
  commitments: [],
  signals: [],
  tomorrow: { label: 'Besok', items: [] },
  forecast: { title: 'Forecast', meta: '', rows: [], target: 0, target_pos: 50, note: '', commit: 0, best: 0, pipeline: 0 },
  pulse: { meta: '', rows: [] },
  renewal: { rows: [] },
  agents_line: { summary_html: '<b>1 agen</b> aktif', feed: [] },
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, status: 200, statusText: 'OK', text: async () => JSON.stringify(today) })))
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

test('renders greeting and queue items from /api/today', async () => {
  render(<UIProvider><TodayScreen /></UIProvider>)
  expect(await screen.findByText('Selamat pagi, Uji.')).toBeInTheDocument()
  expect(screen.getByText('Kirim revisi penawaran')).toBeInTheDocument()
  expect(screen.getByText('1 item')).toBeInTheDocument()
  expect(fetch).toHaveBeenCalledWith('/api/today', expect.anything())
})
