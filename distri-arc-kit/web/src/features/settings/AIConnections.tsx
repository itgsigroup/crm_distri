import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MCPClient, MCPPolicy } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm } from '../../lib/format'
import { useMCPClients, useMCPInfo, useMCPPolicy } from '../../app/queries'

const MODES: [string, string][] = [['api', 'API AI'], ['mcp', 'MCP'], ['both', 'Keduanya']]

/** Analysis path switch (policy llm.routing.mode) — Pengaturan and the Orchestrator header. */
export function ModeSeg() {
  const { data: info } = useMCPInfo()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const set = useMutation({
    mutationFn: (mode: string) => api.put('/policies/llm', { mode }),
    onSuccess: (_r, mode) => {
      toast(`Jalur analisis: ${MODES.find((m) => m[0] === mode)?.[1]} · berlaku di siklus berikutnya`)
      qc.invalidateQueries({ queryKey: ['mcp'] })
      qc.invalidateQueries({ queryKey: ['policies'] })
    },
    onError: (e: Error) => toast(e.message),
  })
  const mode = info?.llm.mode ?? 'both'
  return (
    <div className="seg">
      {MODES.map(([k, l]) => <button key={k} className={mode === k ? 'is-active' : ''} onClick={() => set.mutate(k)}>{l}</button>)}
    </div>
  )
}

/** mcp.permissions switches; allow_send is locked in code. */
export function MCPRules() {
  const { data: pol } = useMCPPolicy()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const set = useMutation({
    mutationFn: (p: MCPPolicy) => api.put<MCPPolicy>('/policies/mcp', p),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['policies', 'mcp'] }),
    onError: (e: Error) => toast(e.message),
  })
  const flip = (k: 'allow_reanalyze' | 'allow_plan_update_proposal') => pol && set.mutate({ ...pol, [k]: !pol[k] })
  return (
    <ul className="rules">
      <li><div><b>Klien MCP boleh memicu analisis ulang</b><span>orchestrator_reanalyze(scope) · hasil masuk antrean, bukan langsung ke dealer · maks {pol?.max_cycles_per_hour ?? 6} siklus/jam</span></div><button className={`sw ${pol?.allow_reanalyze ? 'on' : ''}`} aria-label="toggle" onClick={() => flip('allow_reanalyze')} /></li>
      <li><div><b>Klien MCP boleh mengubah rencana hari ini</b><span>orchestrator_plan_update · setiap perubahan butuh approve Anda</span></div><button className={`sw ${pol?.allow_plan_update_proposal ? 'on' : ''}`} aria-label="toggle" onClick={() => flip('allow_plan_update_proposal')} /></li>
      <li><div><b>Klien MCP boleh mengirim ke dealer</b><span>Tidak pernah. Pengiriman hanya lewat tombol Setujui di aplikasi ini</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
    </ul>
  )
}

function copy(text: string, toast: (m: string) => void) {
  void navigator.clipboard?.writeText(text).then(() => toast('Disalin'), () => toast(text))
}

/** Sheet: MCP clients, create a token (shown once), revoke. */
export function MCPClientsPanel() {
  const { closeSheet, toast } = useFeedback()
  const { data: clients = [] } = useMCPClients()
  const { data: info } = useMCPInfo()
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [scopes, setScopes] = useState<string[]>(['read', 'analyze'])
  const [token, setToken] = useState<string | null>(null)
  const create = useMutation({
    mutationFn: () => api.post<{ token: string; client: MCPClient }>('/mcp/clients', { name, scopes }),
    onSuccess: (r) => {
      setToken(r.token)
      setName('')
      qc.invalidateQueries({ queryKey: ['mcp'] })
    },
    onError: (e: Error) => toast(e.message),
  })
  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/mcp/clients/${id}`),
    onSuccess: () => {
      toast('Token dicabut')
      qc.invalidateQueries({ queryKey: ['mcp'] })
    },
    onError: (e: Error) => toast(e.message),
  })
  const toggle = (s: string) => setScopes(scopes.includes(s) ? scopes.filter((x) => x !== s) : [...scopes, s])
  const config = token && info ? JSON.stringify({ mcpServers: { 'distri-arc': { url: info.endpoint, headers: { Authorization: `Bearer ${token}` } } } }, null, 2) : ''
  return (
    <>
      <SheetHead icon="plug" title="Klien MCP" sub={info ? info.endpoint : 'GSI Orbit sebagai server MCP'} onClose={closeSheet} />
      <div className="sec">
        <h4>Klien terdaftar</h4>
        <ul className="rules">
          {clients.length === 0 && <li><div><b>Belum ada klien</b><span>Buat token untuk Claude Desktop, ChatGPT, atau agent lain</span></div></li>}
          {clients.map((c) => (
            <li key={c.id}>
              <div><b>{c.name}</b><span>{c.scopes.join(' · ')} · {c.token_prefix} · {c.calls_today} panggilan hari ini{c.last_seen_at ? ` · terakhir ${hhmm(c.last_seen_at)}` : ''}</span></div>
              {c.active ? <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => revoke.mutate(c.id)}>Cabut</button> : <Pill tone="neutral">dicabut</Pill>}
            </li>
          ))}
        </ul>
      </div>
      <div className="sec">
        <h4>Buat token</h4>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Nama klien, mis. Claude Desktop Sam" style={{ width: '100%', height: 36, borderRadius: 10, border: '1px solid var(--line)', padding: '0 12px', font: 'inherit', background: 'var(--surface)', color: 'var(--text)' }} />
        <div className="chips" style={{ marginTop: 10 }}>
          {['read', 'analyze', 'orchestrate'].map((s) => <button key={s} className={`chip ${scopes.includes(s) ? 'is-active' : ''}`} onClick={() => toggle(s)}>{s}</button>)}
          <span className="chip" style={{ opacity: 0.5 }} title="Keputusan hanya oleh manusia"><Icon name="lock" style={{ width: 12, height: 12, marginRight: 4, verticalAlign: '-2px' }} />decide</span>
        </div>
        {token && (
          <div className="prev" style={{ marginTop: 12 }}>
            <b>Token (ditampilkan sekali):</b>
            <div className="ep" style={{ marginTop: 6 }}><span style={{ wordBreak: 'break-all' }}>{token}</span><button className="btn ghost" onClick={() => copy(token, toast)}>Salin</button></div>
            <p style={{ fontSize: 12, color: 'var(--text-2)', margin: '10px 0 6px' }}>Claude Desktop · <code>claude_desktop_config.json</code></p>
            <pre style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5, whiteSpace: 'pre-wrap', margin: 0 }}>{config}</pre>
          </div>
        )}
      </div>
      <div className="ft">
        <button className="btn primary" disabled={!name.trim() || scopes.length === 0 || create.isPending} onClick={() => create.mutate()}><Icon name="plug" />Buat token</button>
        <button className="btn quiet" onClick={closeSheet}>Tutup</button>
        <span className="spacer" />
        <span className="pol"><Icon name="lock" />Klien MCP tidak pernah memutuskan atau mengirim ke dealer</span>
      </div>
    </>
  )
}

/** Pengaturan → Koneksi AI (mockup). */
export function AIConnectionsCard() {
  const { data: info } = useMCPInfo()
  const { data: clients = [] } = useMCPClients()
  const { toast } = useFeedback()
  const nav = useNavigate()
  const active = clients.filter((c) => c.active)
  const llm = info?.llm
  const apiOK = llm?.provider === 'anthropic' && llm.api_key
  return (
    <div className="card">
      <div className="card-h"><h2>Koneksi AI</h2><span className="meta">Mesin analisis agen</span></div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 12 }}>
        <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)' }}>Analisis via</span>
        <ModeSeg />
      </div>
      <div className="conn-grid" style={{ gridTemplateColumns: '1fr' }}>
        <button className="cc" onClick={() => toast(apiOK ? `Model ${llm?.model} · cadangan ${llm?.fallback}` : 'Isi ANTHROPIC_API_KEY dan LLM_PROVIDER=anthropic di server; sementara alasan & draft dari template agen')}>
          <div className="ch"><span className="lg" style={{ background: '#D97706' }}>API</span><div><b>API AI langsung</b><small>Claude API ({llm?.model ?? 'claude-sonnet-5-5'}) · cadangan OpenAI · batch analisis tiap jam dari server GSI</small></div></div>
          <div className="cs"><span className={`dot ${apiOK ? 'good' : 'warn'}`} />{apiOK ? 'Terhubung · API key tersimpan di server' : 'Mode template · API key belum diisi'}</div>
        </button>
        <button className="cc" onClick={() => nav('/claude')}>
          <div className="ch"><span className="lg" style={{ background: '#5E5CE6' }}>MCP</span><div><b>MCP · GSI Orbit sebagai server</b><small>Claude (claude.ai, Desktop, Code) membaca dan menganalisis semua data lewat tool MCP — atur di menu MCP Claude</small></div></div>
          <div className="cs"><span className={`dot ${active.length ? 'good' : 'warn'}`} />{active.length ? `Aktif · ${active.length} koneksi: ${active.map((c) => c.name).join(' · ')}` : 'Aktif · belum ada koneksi — buka MCP Claude'}</div>
        </button>
        <button className="cc" onClick={() => toast('GSI Orbit sebagai klien MCP (Odoo MCP) disiapkan di Stage 13')}>
          <div className="ch"><span className="lg" style={{ background: '#1E88E5' }}>MCP</span><div><b>MCP · GSI Orbit sebagai klien</b><small>Memakai MCP server lain: Odoo MCP, WhatsApp bridge, Getcontact (manual)</small></div></div>
          <div className="cs"><span className="dot warn" />Odoo MCP belum dipasang · sementara lewat API Odoo</div>
        </button>
      </div>
      <div className="ep" style={{ marginTop: 12 }}><span>{info?.endpoint ?? '/mcp'}</span><button className="btn ghost" onClick={() => info && copy(info.endpoint, toast)}>Salin</button></div>
      <p style={{ fontSize: 12, color: 'var(--text-3)', margin: '8px 0 0', lineHeight: 1.5 }}>Tool MCP: <code style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5 }}>{(info?.tools ?? []).map((t) => t.name + (t.scope === 'decide' ? ' (human-only)' : '')).join(' / ')}</code></p>
      <ul className="rules" style={{ marginTop: 10 }}>
        <li><div><b>Analisis per jam</b><span>Batch otomatis 06.00–20.00: API AI bila tersedia, MCP sebagai jalur tambahan — hasilnya sama-sama masuk antrean Keputusan</span></div><Pill tone="good" icon="check">Aktif</Pill></li>
        <li><div><b>Pertanyaan ad-hoc (⌘K)</b><span>Dijawab lewat jalur yang aktif; sumber data selalu dari server GSI, bukan dari cache klien</span></div><Pill tone="good" icon="check">Aktif</Pill></li>
        <li><div><b>Keputusan tetap manusia</b><span>API maupun MCP hanya mengusulkan; setujui / tolak ada di Pusat kendali</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
      </ul>
    </div>
  )
}
