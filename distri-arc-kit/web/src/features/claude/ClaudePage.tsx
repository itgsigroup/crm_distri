import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MCPClient, MCPPolicy } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { useMCPCalls, useMCPClients, useMCPInfo, useMCPPolicy, useMe } from '../../app/queries'
import { useMore } from '../../components/More'
import { Schedules } from './Schedules'
import { PROMPTS, SCOPE } from './shared'

function copy(text: string, toast: (m: string) => void) {
  void navigator.clipboard?.writeText(text).then(() => toast('Disalin'), () => toast(text))
}

function Code({ children, onCopy }: { children: string; onCopy: () => void }) {
  return (
    <div className="cl-code">
      <pre>{children}</pre>
      <button className="btn ghost" onClick={onCopy}><Icon name="doc" />Salin</button>
    </div>
  )
}

function Connect({ endpoint }: { endpoint: string }) {
  const [tab, setTab] = useState<'web' | 'code' | 'token'>('web')
  const { toast } = useFeedback()
  return (
    <div className="card">
      <div className="card-h"><h2>Hubungkan Claude</h2><span className="meta">Login GSI Orbit + izin, tanpa menyalin token</span></div>
      <div className="seg" style={{ marginBottom: 12 }}>
        <button className={tab === 'web' ? 'is-active' : ''} onClick={() => setTab('web')}>claude.ai &amp; Claude Desktop</button>
        <button className={tab === 'code' ? 'is-active' : ''} onClick={() => setTab('code')}>Claude Code</button>
        <button className={tab === 'token' ? 'is-active' : ''} onClick={() => setTab('token')}>Token manual</button>
      </div>
      {tab === 'web' && (
        <ol className="cl-steps">
          <li>Di <b>claude.ai</b> (atau aplikasi Claude Desktop) buka <b>Settings → Connectors</b>, lalu <b>Add custom connector</b>. Untuk akun Team/Enterprise, owner menambahkannya di <b>Admin settings → Connectors</b>.</li>
          <li>Isi <b>Name</b>: <code>GSI Orbit</code> dan <b>Remote MCP server URL</b>:<Code onCopy={() => copy(endpoint, toast)}>{endpoint}</Code></li>
          <li>Klik <b>Add</b>, lalu <b>Connect</b>. Halaman GSI Orbit terbuka: login, periksa izinnya, lalu <b>Izinkan</b>.</li>
          <li>Di percakapan baru, aktifkan <b>GSI Orbit</b> di menu alat (ikon <b>+</b> / <b>Search and tools</b>) lalu pakai salah satu contoh prompt di bawah.</li>
        </ol>
      )}
      {tab === 'code' && (
        <ol className="cl-steps">
          <li>Tambahkan server (sekali):<Code onCopy={() => copy(`claude mcp add --transport http distri-arc ${endpoint}`, toast)}>{`claude mcp add --transport http distri-arc ${endpoint}`}</Code></li>
          <li>Di Claude Code jalankan <code>/mcp</code>, pilih <b>distri-arc</b> → <b>Authenticate</b>. Browser membuka GSI Orbit: login lalu <b>Izinkan</b>.</li>
          <li>Tanya langsung, mis. “pakai distri-arc, ringkas kondisi orbit semua cabang”.</li>
        </ol>
      )}
      {tab === 'token' && (
        <ol className="cl-steps">
          <li>Untuk klien yang tidak mendukung login OAuth (agent sendiri, Claude API, ChatGPT): buat token di kartu <b>Token manual</b> di bawah (hanya CEO). Token ditampilkan sekali.</li>
          <li>Claude Code dengan token:<Code onCopy={() => copy(`claude mcp add --transport http distri-arc ${endpoint} --header "Authorization: Bearer <TOKEN>"`, toast)}>{`claude mcp add --transport http distri-arc ${endpoint} --header "Authorization: Bearer <TOKEN>"`}</Code></li>
          <li>Claude API (MCP connector): <code>{`mcp_servers: [{ type: "url", url: "${endpoint}", name: "distri-arc", authorization_token: "<TOKEN>" }]`}</code></li>
        </ol>
      )}
    </div>
  )
}

function Connections() {
  const { data: all = [] } = useMCPClients()
  const clients = all.filter((c) => c.kind !== 'schedule') // scheduled analysis has its own card
  const { data: me } = useMe()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/mcp/clients/${id}`),
    onSuccess: () => { toast('Koneksi dicabut · Claude harus diizinkan ulang'); qc.invalidateQueries({ queryKey: ['mcp'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const active = clients.filter((c) => c.active)
  const [shown, more] = useMore(clients.slice().sort((a, b) => Number(b.active) - Number(a.active)), 8)
  return (
    <div className="card">
      <div className="card-h"><h2>Koneksi</h2><span className="meta">{active.length} aktif · dicabut kapan saja</span></div>
      {clients.length === 0 ? <p className="cl-empty">Belum ada Claude yang terhubung. Ikuti langkah di atas.</p> : (
        <ul className="rules">
          {shown.map((c: MCPClient) => (
            <li key={c.id} style={c.active ? undefined : { opacity: 0.5 }}>
              <div>
                <b>{c.name}</b>
                <span>{c.kind === 'oauth' ? 'Login OAuth' : 'Token manual'} · {c.scopes.map((s) => SCOPE[s]?.[0] ?? s).join(', ')} · {c.calls_today} panggilan hari ini{c.last_seen_at ? ` · terakhir ${shortDate(c.last_seen_at)} ${hhmm(c.last_seen_at)}` : ' · belum dipakai'}</span>
              </div>
              <span style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                <Pill tone={c.kind === 'oauth' ? 'accent' : 'neutral'}>{c.kind === 'oauth' ? 'Claude' : c.token_prefix}</Pill>
                {c.active ? (me?.edit_policies && <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => { if (window.confirm(`Cabut koneksi ${c.name}?`)) revoke.mutate(c.id) }}>Cabut</button>) : <Pill tone="neutral">dicabut</Pill>}
              </span>
            </li>
          ))}
        </ul>
      )}
      {more}
    </div>
  )
}

function ManualToken({ endpoint }: { endpoint: string }) {
  const { data: me } = useMe()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [name, setName] = useState('')
  const [scopes, setScopes] = useState<string[]>(['read', 'analyze'])
  const [token, setToken] = useState('')
  const create = useMutation({
    mutationFn: () => api.post<{ token: string }>('/mcp/clients', { name, scopes }),
    onSuccess: (r) => { setToken(r.token); setName(''); qc.invalidateQueries({ queryKey: ['mcp'] }) },
    onError: (e: Error) => toast(e.message),
  })
  if (!me?.edit_policies) return null
  const flip = (s: string) => setScopes(scopes.includes(s) ? scopes.filter((x) => x !== s) : [...scopes, s])
  const desktop = token ? JSON.stringify({ mcpServers: { 'distri-arc': { command: 'npx', args: ['-y', 'mcp-remote', endpoint, '--header', 'Authorization:Bearer ${ARC_TOKEN}'], env: { ARC_TOKEN: token } } } }, null, 2) : ''
  return (
    <div className="card">
      <div className="card-h"><h2>Token manual</h2><span className="meta">Untuk klien tanpa login OAuth</span></div>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <input className="cl-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Nama klien, mis. Agent laporan mingguan" aria-label="Nama klien" />
        <button className="btn primary" disabled={!name.trim() || !scopes.length || create.isPending} onClick={() => create.mutate()}><Icon name="plug" />Buat token</button>
      </div>
      <div className="chips" style={{ marginTop: 10 }}>
        {['read', 'analyze', 'orchestrate'].map((s) => <button key={s} className={`chip ${scopes.includes(s) ? 'is-active' : ''}`} onClick={() => flip(s)}>{SCOPE[s][0]}</button>)}
      </div>
      {token && (
        <div className="cl-token">
          <b>Token (ditampilkan sekali — simpan sekarang)</b>
          <Code onCopy={() => copy(token, toast)}>{token}</Code>
          <span>Claude Desktop lewat <code>mcp-remote</code> · <code>claude_desktop_config.json</code></span>
          <Code onCopy={() => copy(desktop, toast)}>{desktop}</Code>
        </div>
      )}
    </div>
  )
}

function Rules() {
  const { data: pol } = useMCPPolicy()
  const { data: me } = useMe()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const set = useMutation({
    mutationFn: (p: MCPPolicy) => api.put<MCPPolicy>('/policies/mcp', p),
    onSuccess: () => { toast('Izin MCP disimpan'); qc.invalidateQueries({ queryKey: ['policies', 'mcp'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const edit = !!me?.edit_policies
  const flip = (k: 'allow_reanalyze' | 'allow_plan_update_proposal' | 'mask_pii_in_read') => pol && edit && set.mutate({ ...pol, [k]: !pol[k] })
  return (
    <div className="card">
      <div className="card-h"><h2>Izin &amp; batas</h2><span className="meta">{edit ? 'Berlaku untuk semua koneksi' : 'Diubah oleh pemegang hak kebijakan'}</span></div>
      <ul className="rules">
        <li><div><b>Samarkan nomor & email</b><span>Nomor WhatsApp, telepon, dan email disamarkan sebelum dikirim ke Claude</span></div><button className={`sw ${pol?.mask_pii_in_read ? 'on' : ''}`} disabled={!edit} aria-label="Samarkan PII" onClick={() => flip('mask_pii_in_read')} /></li>
        <li><div><b>Boleh memicu analisis ulang</b><span>orchestrator_reanalyze · hasil masuk antrean Keputusan · maks
          {edit && pol ? <select className="cl-num" value={pol.max_cycles_per_hour} onChange={(e) => set.mutate({ ...pol, max_cycles_per_hour: Number(e.target.value) })}>{[1, 3, 6, 10, 20, 30].map((n) => <option key={n}>{n}</option>)}</select> : ` ${pol?.max_cycles_per_hour ?? 6} `}
          siklus/jam</span></div><button className={`sw ${pol?.allow_reanalyze ? 'on' : ''}`} disabled={!edit} aria-label="Analisis ulang" onClick={() => flip('allow_reanalyze')} /></li>
        <li><div><b>Boleh mengusulkan perubahan rencana hari ini</b><span>orchestrator_plan_update · setiap perubahan butuh approve</span></div><button className={`sw ${pol?.allow_plan_update_proposal ? 'on' : ''}`} disabled={!edit} aria-label="Ubah rencana" onClick={() => flip('allow_plan_update_proposal')} /></li>
        <li><div><b>Memutuskan atau mengirim ke dealer</b><span>Tidak pernah. Claude hanya membaca dan mengusulkan; setujui/kirim hanya di aplikasi</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
        <li><div><b>Batas panggilan</b><span>60 panggilan/menit per koneksi · token akses 1 jam, diperbarui otomatis · koneksi berlaku 90 hari sejak terakhir dipakai</span></div><Pill tone="neutral" icon="lock">Tetap</Pill></li>
      </ul>
    </div>
  )
}

function Tools() {
  const { data: info } = useMCPInfo()
  const groups = ['read', 'analyze', 'orchestrate', 'decide']
  return (
    <div className="card">
      <div className="card-h"><h2>Tool MCP</h2><span className="meta">{info?.tools.length ?? 0} tool · Claude memilih sendiri</span></div>
      {groups.map((g) => {
        const ts = (info?.tools ?? []).filter((t) => t.scope === g)
        if (!ts.length) return null
        return (
          <div key={g} className="cl-tools">
            <h4>{SCOPE[g][0]} <span>· {SCOPE[g][1]}</span></h4>
            <ul>{ts.map((t) => <li key={t.name}><code>{t.name}</code><span>{t.description}</span></li>)}</ul>
          </div>
        )
      })}
    </div>
  )
}

function Calls() {
  const { data: calls = [] } = useMCPCalls()
  const tone = (s: string) => (s === 'ok' ? 'good' : s === 'human_only' || s === 'rate_limited' ? 'warn' : 'bad')
  return (
    <div className="card">
      <div className="card-h"><h2>Log panggilan</h2><span className="meta">20 terakhir · tercatat di audit</span></div>
      {calls.length === 0 ? <p className="cl-empty">Belum ada panggilan.</p> : (
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Waktu</th><th>Koneksi</th><th>Tool</th><th>Hasil</th><th>Status</th></tr></thead>
            <tbody>{calls.map((c) => (
              <tr key={c.id}><td className="mono">{shortDate(c.created_at)} {hhmm(c.created_at)}</td><td>{c.client_name ?? '—'}</td><td><code>{c.tool}</code></td>
                <td style={{ maxWidth: 280 }}>{c.result_summary}{c.duration_ms != null && <span className="mono"> · {c.duration_ms} ms</span>}</td><td><Pill tone={tone(c.status)}>{c.status}</Pill></td></tr>
            ))}</tbody>
          </table>
        </div>
      )}
    </div>
  )
}

/** Sidebar → MCP Claude: connect Claude to analyse all of GSI Orbit's data (read-only; decisions stay here). */
export function ClaudePage() {
  const { data: info } = useMCPInfo()
  const { data: clients = [] } = useMCPClients()
  const { toast } = useFeedback()
  const endpoint = info?.endpoint ?? ''
  const active = clients.filter((c) => c.active && c.kind !== 'schedule').length
  return (
    <div className="stack">
      <div className="card cl-hero">
        <span className="cl-logo"><Icon name="spark" /></span>
        <div style={{ flex: 1, minWidth: 0 }}>
          <h2>Claude menganalisis semua data GSI Orbit</h2>
          <p>Lewat MCP, Claude membaca dealer, penjualan, piutang, stok, dan chat (nomor disamarkan) langsung dari server GSI — lalu menjawab, membandingkan, dan menyusun rencana. Claude <b>tidak pernah</b> memutuskan atau mengirim ke dealer.</p>
          <div className="cl-status">
            <Pill tone={info?.enabled ? 'good' : 'bad'} icon={info?.enabled ? 'check' : 'alert'}>{info?.enabled ? 'Server MCP aktif' : 'Server MCP mati'}</Pill>
            <Pill tone="good" icon="lock">Login OAuth 2.1 + PKCE</Pill>
            <Pill tone="neutral">{info?.tools.length ?? 0} tool</Pill>
            <Pill tone={active ? 'accent' : 'neutral'}>{active} koneksi aktif</Pill>
          </div>
          <div className="ep" style={{ marginTop: 10 }}><span>{endpoint}</span><button className="btn ghost" onClick={() => copy(endpoint, toast)}>Salin URL</button></div>
        </div>
      </div>
      <Schedules />
      <div className="ai-grid">
        <div className="stack">
          <Connect endpoint={endpoint} />
          <div className="card">
            <div className="card-h"><h2>Contoh prompt analisis</h2><span className="meta">Salin ke Claude</span></div>
            <ul className="cl-prompts">{PROMPTS.map((p) => <li key={p}><span>{p}</span><button className="btn quiet" onClick={() => copy(p, toast)}><Icon name="doc" />Salin</button></li>)}</ul>
          </div>
          <Calls />
        </div>
        <div className="stack">
          <Connections />
          <Rules />
          <ManualToken endpoint={endpoint} />
          <Tools />
        </div>
      </div>
    </div>
  )
}
