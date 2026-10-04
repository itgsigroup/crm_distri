import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Pipeline } from '../api/types'
import { UIProvider } from '../state/UIProvider'
import { PipelineScreen } from './Pipeline'

const fixture: Pipeline = {
  sync: { connected: true, title: 'Odoo CRM · test-db', meta: 'sinkron 2 arah', pill: { k: 'good', t: '1 opportunity' }, source_label: 'odoo' },
  stages: [
    {
      id: 1, name: 'Berkualifikasi', total: 1.2e9, won: false,
      cards: [{
        id: 'opp-1', account_id: 'acc-1', opp: 'Videotron Lobby', account: 'PT Contoh', value: 1.2e9, prob: 'at 70%',
        closing: '15 Okt 2026', tags: ['Videotron'], prio: 2, activity: 'warn', owner_initials: 'AN',
        health: 64, signal: 'sunyi 9 hari', note: '', action: null, won: false,
      }],
    },
    { id: 2, name: 'Won', total: 0, won: true, cards: [] },
  ],
  field: [], field_title: '1 deal terbuka',
  inference: [], weighted: { sales: 0, arc: 0, note: '' },
  winloss: { meta: '', rows: [] }, tenders: { meta: '', items: [] }, team: [],
}

describe('PipelineScreen', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(fixture), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  })
  afterEach(() => vi.unstubAllGlobals())

  it('renders kanban columns and cards from /api/pipeline', async () => {
    render(<UIProvider><PipelineScreen /></UIProvider>)
    expect(await screen.findByText('Berkualifikasi')).toBeInTheDocument()
    expect(screen.getByText('Videotron Lobby')).toBeInTheDocument()
    expect(screen.getByText('PT Contoh')).toBeInTheDocument()
    expect(screen.getByText('Health 64')).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/pipeline', expect.objectContaining({ method: 'GET' }))
  })
})
