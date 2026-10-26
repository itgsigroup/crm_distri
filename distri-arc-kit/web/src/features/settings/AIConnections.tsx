import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MCPClient, MCPPolicy } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm } from '../../lib/format'
import { useMCPClients, useMCPInfo, useMCPPolicy, useMe } from '../../app/queries'

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

function copySecret(text: string, toast: (m: string) => void) {
  if (!navigator.clipboard?.writeText) {
    toast('Clipboard tidak tersedia; salin dari blok secara manual')
    return
  }
  void navigator.clipboard.writeText(text).then(() => toast('Disalin'), () => toast('Gagal menyalin; salin dari blok secara manual'))
}

/** Manage a manual token connection: edit its label/scopes or rotate its secret. */
export function MCPClientEditor({ client }: { client: MCPClient }) {
  const { closeSheet, toast } = useFeedback()
  const { data: info } = useMCPInfo()
  const qc = useQueryClient()
  const [name, setName] = useState(client.name)
  const [scopes, setScopes] = useState<string[]>([...client.scopes])
  const [token, setToken] = useState('')
  const update = useMutation({
    mutationFn: () => api.patch<{ client: MCPClient }>(`/mcp/clients/${client.id}`, { name, scopes }),
    onSuccess: () => {
      toast('Nama dan izin koneksi diperbarui')
      qc.invalidateQueries({ queryKey: ['mcp'] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const rotate = useMutation({
    mutationFn: () => api.post<{ token: string; client: MCPClient }>(`/mcp/clients/${client.id}/rotate`),
    onSuccess: (r) => {
      setToken(r.token)
      qc.invalidateQueries({ queryKey: ['mcp'] })
    },
    onError: (e: Error) => toast(e.message),
  })
  const toggle = (scope: string) => setScopes((current) => current.includes(scope) ? current.filter((s) => s !== scope) : [...current, scope])
  const hasEdits = name !== client.name || scopes.join('|') !== client.scopes.join('|')
  const hermes = token && info ? `mcp_servers:\n  distri_crm:\n    url: ${JSON.stringify(info.endpoint)}\n    headers:\n      Authorization: ${JSON.stringify(`Bearer ${token}`)}\n    connect_timeout: 60\n    timeout: 180` : ''
  return (
    <>
      <SheetHead icon="plug" title={`Edit · ${client.name}`} sub="Untuk koneksi token manual seperti Hermes Agent" onClose={closeSheet} />
      <div className="sec">
        <h4>Nama koneksi</h4>
        <input value={name} onChange={(e) => setName(e.target.value)} maxLength={120} placeholder="Nama koneksi" style={{ width: '100%', height: 36, borderRadius: 10, border: '1px solid var(--line)', padding: '0 12px', font: 'inherit', background: 'var(--surface)', color: 'var(--text)' }} />
        <h4 style={{ marginTop: 16 }}>Izin token</h4>
        <div className="chips">
          {['read', 'analyze', 'orchestrate'].map((scope) => <button type="button" key={scope} className={`chip ${scopes.includes(scope) ? 'is-active' : ''}`} onClick={() => toggle(scope)}>{scope}</button>)}
          <span className="chip" style={{ opacity: 0.5 }} title="Keputusan hanya oleh manusia"><Icon name="lock" style={{ width: 12, height: 12, marginRight: 4, verticalAlign: '-2px' }} />decide</span>
        </div>
        <p style={{ fontSize: 12, color: 'var(--text-2)', lineHeight: 1.5 }}>Perubahan izin langsung berlaku pada token yang sama. Izin <code>decide</code> hanya dimiliki manusia.</p>
      </div>
      <div className="sec">
        <h4>Ganti token rahasia</h4>
        <p style={{ fontSize: 12, color: 'var(--text-2)', lineHeight: 1.5 }}>Rotasi langsung menonaktifkan token lama. Perbarui konfigurasi Hermes setelah token baru muncul.</p>
        {hasEdits && <p style={{ fontSize: 12, color: 'var(--text-2)', margin: '0 0 8px' }}>Simpan perubahan nama atau izin sebelum merotasi token.</p>}
        {!token ? <button className="btn ghost" disabled={rotate.isPending || !client.active || hasEdits} onClick={() => { if (window.confirm(`Rotasi token ${client.name}? Token lama langsung tidak berlaku.`)) rotate.mutate() }}>{rotate.isPending ? 'Merotasi…' : 'Rotasi token'}</button> : (
          <div className="prev" style={{ marginTop: 10 }}>
            <b>Token baru (ditampilkan sekali):</b>
            <div className="ep" style={{ marginTop: 6 }}><span style={{ wordBreak: 'break-all' }}>{token}</span><button className="btn ghost" onClick={() => copySecret(token, toast)}>Salin</button></div>
            <p style={{ fontSize: 12, color: 'var(--text-2)', margin: '12px 0 6px' }}>Perbarui <code>~/.hermes/config.yaml</code></p>
            <pre style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', margin: 0 }}>{hermes}</pre>
            <button className="btn ghost" style={{ marginTop: 8 }} onClick={() => copySecret(hermes, toast)}>Salin konfigurasi Hermes</button>
          </div>
        )}
      </div>
      <div className="ft">
        <button className="btn primary" disabled={!name.trim() || scopes.length === 0 || update.isPending || (name === client.name && scopes.join('|') === client.scopes.join('|'))} onClick={() => update.mutate()}><Icon name="check" />Simpan perubahan</button>
        <button className="btn quiet" onClick={closeSheet}>Tutup</button>
        <span className="spacer" />
        <span className="pol"><Icon name="lock" />Rahasia token hanya ditampilkan saat dibuat atau dirotasi</span>
      </div>
    </>
  )
}

/** Sheet: MCP clients, create a token (shown once), revoke. */
export function MCPClientsPanel() {
  const { closeSheet, openSheet, toast } = useFeedback()
  const { data: clients = [] } = useMCPClients()
  const { data: info } = useMCPInfo()
  const { data: me } = useMe()
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
              {c.active ? <span style={{ display: 'flex', gap: 6 }}>{c.kind === 'bearer' && me?.edit_policies && <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<MCPClientEditor client={c} />)}>Edit</button>}<button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => revoke.mutate(c.id)}>Cabut</button></span> : <Pill tone="neutral">dicabut</Pill>}
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
            <div className="ep" style={{ marginTop: 6 }}><span style={{ wordBreak: 'break-all' }}>{token}</span><button className="btn ghost" onClick={() => copySecret(token, toast)}>Salin</button></div>
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
        <button className="cc" onClick={() => nav('/pengaturan/api-ai')}>
          <div className="ch"><span className="lg" style={{ background: '#D97706' }}>API</span><div><b>API &amp; model AI</b><small>Claude API atau ChatGPT API sebagai opsi kedua bila Claude lewat MCP tidak menjawab · kunci, model, total pembayaran token</small></div></div>
          <div className="cs"><span className={`dot ${apiOK ? 'good' : 'warn'}`} />{apiOK ? 'Terhubung · API key tersimpan di server' : 'Mode template · API key belum diisi'}</div>
        </button>
        <button className="cc" onClick={() => nav('/claude')}>
          <div className="ch"><span className="lg" style={{ background: '#5E5CE6' }}>MCP</span><div><b>MCP · GSI Orbit sebagai server</b><small>Claude, Hermes Agent, dan klien MCP lain dapat memakai tool CRM — atur token dan koneksi di menu MCP Claude</small></div></div>
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
