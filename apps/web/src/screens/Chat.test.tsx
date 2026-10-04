import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatListItem, ChatThread } from '../api/types'
import { UIProvider } from '../state/UIProvider'
import { ChatScreen } from './Chat'

const list: { items: ChatListItem[]; unread: number } = {
  unread: 2,
  items: [
    { id: 'c-1', type: 'cust', name: 'Pelanggan Satu', sub: 'Instansi A', via: 'Andi', time: '12.40', unread: 2, tag: { k: 'accent', t: 'Tag A' }, last: 'Halo', private: false, initials: 'PS' },
    { id: 'c-priv', type: 'internal', name: 'Karyawan Dua', sub: 'Teknisi', via: 'Andi', time: '11.05', unread: 0, tag: { k: 'neutral', t: 'Tidak dibaca' }, last: '', private: true, initials: 'KD' },
  ],
}

const custThread: ChatThread = {
  id: 'c-1', type: 'cust', name: 'Pelanggan Satu', sub: 'Instansi A', via: 'Andi', initials: 'PS',
  via_note: 'pelanggan · dibaca ARC', private: false,
  messages: [
    { day: 'Hari ini' },
    { id: 1, f: 'in', t: 'Revisi penawarannya jadi hari ini?', tm: '12.31', ann: { k: 'warn', t: 'Komitmen jatuh tempo', act: 'Buat tugas', act_id: 'a1' } },
  ],
  suggestions: ['Konfirmasi besok'],
  policy_line: 'Dikirim dari nomor Andi',
  context: { kind: 'customer', extracted: [], open_commitments: [] },
}

const privThread: ChatThread = {
  id: 'c-priv', type: 'internal', name: 'Karyawan Dua', sub: 'Teknisi', via: 'Andi', initials: 'KD',
  via_note: 'internal · tidak dibaca', private: true,
  private_note: { title: 'Chat pribadi antar karyawan tidak dibaca ARC.', body: 'Nomor ini terdaftar sebagai internal.' },
  messages: [], suggestions: [], policy_line: '',
  context: { kind: 'internal', note: 'Isi tidak dibaca.', internal_members: [] },
}

function mockFetch() {
  const routes: Record<string, unknown> = {
    '/api/chat/threads?type=all': list,
    '/api/chat/threads/c-1': custThread,
    '/api/chat/threads/c-priv': privThread,
  }
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    const body = routes[url]
    return new Response(body === undefined ? '{"error":"not found"}' : JSON.stringify(body), { status: body === undefined ? 404 : 200 })
  })
}

describe('ChatScreen', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetch())
  })
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    window.location.hash = ''
  })

  it('renders the first thread with messages and annotations', async () => {
    window.location.hash = '#chat'
    render(<UIProvider><ChatScreen /></UIProvider>)
    expect(await screen.findByText('Revisi penawarannya jadi hari ini?')).toBeInTheDocument()
    expect(screen.getByText(/Komitmen jatuh tempo/, { selector: '.ann' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Buat tugas' })).toBeInTheDocument()
    expect(screen.getByText('Balas sebagai Andi')).toBeInTheDocument()
    expect(screen.getByText('Pelanggan', { selector: '.chat-sec' })).toBeInTheDocument()
  })

  it('shows the privacy note for a private thread', async () => {
    window.location.hash = '#chat/c-priv'
    render(<UIProvider><ChatScreen /></UIProvider>)
    expect(await screen.findByText('Chat pribadi antar karyawan tidak dibaca ARC.')).toBeInTheDocument()
    expect(screen.getByText('Nomor ini terdaftar sebagai internal.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Lihat aturan' })).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('Tulis balasan…')).not.toBeInTheDocument()
  })
})
