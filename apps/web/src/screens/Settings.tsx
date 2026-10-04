// Pengaturan — ported from mockup <section id="screen-conn"> plus renderInternal,
// int-form, sus ok/no, data-sw toggles, setHist, QR, newkey and copyText.
import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { api, useApi } from '../api/client'
import type {
  ConnectorCard, McpTool, SettingsAI, SettingsAPI, SettingsInternal, SettingsSources, SettingsWhatsApp, Toast, WaNumber,
} from '../api/types'
import { Html, Icon, Loading, Pill } from '../components/ui'
import { fmtNum } from '../lib/format'
import { useUI } from '../state/ui'

// App.tsx renders the conn view tabs from this constant (kept here by contract).
// oxlint-disable-next-line react/only-export-components
export const CONN_TABS: [string, string][] = [['sec-sumber', 'Sumber sinyal'], ['sec-wa', 'WhatsApp'], ['sec-int', 'Nomor internal'], ['sec-ai', 'AI & model'], ['sec-api', 'MCP & API']]

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))

// Mutating call: toast the server message (or the error), then refresh all data.
function useMutate() {
  const { toast, refresh } = useUI()
  return async <T extends Partial<Toast>>(p: Promise<T>): Promise<T | null> => {
    try {
      const r = await p
      if (r?.toast) toast(r.toast)
      return r
    } catch (e) {
      toast(errMsg(e))
      return null
    } finally {
      refresh()
    }
  }
}

// Mockup copyText: clipboard with a manual-copy fallback toast.
function useCopy() {
  const { toast } = useUI()
  return (t: string) => {
    const fail = () => toast('Salin manual: ' + t.slice(0, 40) + '…')
    try {
      navigator.clipboard.writeText(t).then(() => toast('Disalin')).catch(fail)
    } catch {
      fail()
    }
  }
}

const enc = encodeURIComponent

export function SettingsScreen() {
  const { route } = useUI()
  const tab = route.param || 'sec-sumber'
  return (
    <section className="screen" id="screen-conn">
      {tab === 'sec-sumber' && <Sources />}
      {tab === 'sec-wa' && <WhatsApp />}
      {tab === 'sec-int' && <Internal />}
      {tab === 'sec-ai' && <AI />}
      {tab === 'sec-api' && <ApiTab />}
    </section>
  )
}

// Switch toggle (.sw). `small` renders the 36×22 variant used in group lists; the CSS
// keys on the literal inline style string, so it is set as a raw attribute.
function Sw({ on, onToggle, small, label }: { on: boolean; onToggle: () => void; small?: boolean; label?: string }) {
  return (
    <button className={'sw' + (on ? ' on' : '')} aria-label={label || 'toggle'} aria-pressed={on} onClick={onToggle}
      ref={small ? el => { el?.setAttribute('style', 'margin-left:auto;width:36px;height:22px') } : undefined} />
  )
}

// ---------- Sumber sinyal ----------
function Cc({ c }: { c: ConnectorCard }) {
  return (
    <button className={'cc' + (c.active ? ' is-active' : '')}>
      <div className="ch">
        <span className="lg" style={{ background: c.color, ...(c.text_color ? { color: c.text_color } : {}) }}>{c.logo}</span>
        <div><b>{c.name}</b><small>{c.sub}</small></div>
      </div>
      <div className="cs">
        <span className={'dot' + (c.dot === 'off' ? '' : ' ' + c.dot)} style={c.dot === 'off' ? { background: 'var(--line-strong)' } : undefined} />
        {c.status}
      </div>
    </button>
  )
}

function Sources() {
  const { data, error } = useApi<SettingsSources>('/api/settings/sources')
  if (!data) return <Loading error={error} />
  return (
    <div className="card conn-tab" style={{ marginBottom: 16 }} id="sec-sumber">
      <div className="card-h"><h2>Sumber sinyal</h2><span className="meta">Semua lewat MCP · tidak ada integrasi kustom</span></div>
      <div className="conn-grid">{data.sources.map(c => <Cc key={c.id} c={c} />)}</div>
      <div className="card-h" style={{ marginTop: 20 }}><h2 style={{ fontSize: 14 }}>Identifikasi nomor masuk</h2><span className="meta">Hanya untuk nomor yang menghubungi kita lebih dulu · UU PDP</span></div>
      <div className="conn-grid">{data.identity.map(c => <Cc key={c.id} c={c} />)}</div>
      {data.getcontact_note && <Html as="p" style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 12, lineHeight: 1.55 }} html={data.getcontact_note} />}
    </div>
  )
}

// ---------- WhatsApp ----------
const HIST_DAYS = [30, 60, 90, 180]

function NumberStatus({ n }: { n: WaNumber }) {
  const mutate = useMutate()
  let el: ReactNode
  switch (n.status) {
    case 'connected': el = <span className="pill good"><Icon n="i-check" />Terhubung</span>; break
    case 'pairing': el = <span className="pill warn">Menunggu scan QR</span>; break
    case 'disconnected': el = <span className="pill bad">Terputus</span>; break
    default:
      el = (
        <button className="btn primary" style={{ height: 28, fontSize: 12 }} onClick={() => mutate(api.post<Toast>('/api/wa/sessions/' + enc(n.id) + '/link'))}>
          <Icon n="i-qr" />Tautkan
        </button>
      )
  }
  return <div className="st">{el}<small>{n.note}</small></div>
}

function WhatsApp() {
  const { data, error, setData } = useApi<SettingsWhatsApp>('/api/settings/whatsapp')
  const { toast, refresh } = useUI()
  const mutate = useMutate()
  if (!data) return <Loading error={error} />

  const saveSettings = async (body: { mode?: SettingsWhatsApp['mode']; history_days?: number }, msg?: string) => {
    try {
      const r = await api.post<SettingsWhatsApp>('/api/settings/whatsapp', body)
      if (r) setData(r)
      if (msg) toast(msg)
    } catch (e) {
      toast(errMsg(e))
    } finally {
      refresh()
    }
  }
  const toggleRule = (id: string, enabled: boolean) => {
    setData({ ...data, rules: data.rules.map(r => (r.id === id ? { ...r, enabled } : r)) })
    mutate(api.post<Toast>('/api/settings/privacy/' + enc(id), { enabled }))
  }
  const toggleGroup = (id: string, read: boolean) => {
    setData({ ...data, groups: data.groups.map(g => (g.id === id ? { ...g, read } : g)) })
    mutate(api.post<Toast>('/api/chat/groups/' + enc(id) + '/policy', { read }))
  }

  const ruleLi = (r: SettingsWhatsApp['rules'][number]) => (
    <li key={r.id}>
      <div><b>{r.title}</b><span>{r.detail}</span></div>
      {r.locked
        ? <span className="pill neutral"><Icon n="i-lock" />Terkunci</span>
        : <Sw on={r.enabled} onToggle={() => toggleRule(r.id, !r.enabled)} label={r.title} />}
    </li>
  )
  // The mockup places the group list after the first three rules.
  const split = Math.min(3, data.rules.length)
  const p = data.pairing
  const x = data.extracted

  return (
    <div className="wa conn-tab" id="sec-wa">
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>WhatsApp</h2><span className="meta">Cara menghubungkan</span></div>
          <div className="opt">
            <button className={data.mode === 'cloud' ? 'is-active' : ''} onClick={() => saveSettings({ mode: 'cloud' }, 'Mode resmi dipilih · butuh akun Meta Business & nomor bisnis')}>
              <b>WhatsApp Business Platform <span className="pill good">Resmi · disarankan</span></b>
              <p>Satu nomor bisnis per cabang lewat Cloud API Meta. Stabil, tidak berisiko blokir, riwayat pesan tersimpan di sisi kita. Konsekuensi: sales memakai nomor bisnis, bukan nomor pribadi.</p>
            </button>
            <button className={data.mode === 'bridge' ? 'is-active' : ''} onClick={() => saveSettings({ mode: 'bridge' }, 'Mode tautkan perangkat dipilih · baca peringatan risiko')}>
              <b>Tautkan perangkat (QR) <span className="pill warn">Tidak resmi</span></b>
              <p>Nomor pribadi sales ditautkan seperti WhatsApp Web. Cepat dipasang, tapi bergantung protokol tidak resmi dan bisa terputus atau diblokir Meta. Cocok untuk uji coba 30 hari, bukan produksi.</p>
            </button>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap', padding: '12px 14px', borderRadius: 12, background: 'var(--surface-2)', border: '1px solid var(--line)', marginBottom: 14 }}>
            <div><b style={{ fontWeight: 600, fontSize: 13.5, display: 'block' }}>Riwayat awal yang ditarik</b><span style={{ fontSize: 12, color: 'var(--text-3)' }}>Berlaku untuk nomor yang baru ditautkan</span></div>
            <div className="seg" style={{ marginLeft: 'auto' }}>
              {HIST_DAYS.map(d => (
                <button key={d} className={data.history_days === d ? 'is-active' : ''} onClick={() => saveSettings({ history_days: d })}>{d} hari</button>
              ))}
            </div>
            <Html as="div" style={{ flexBasis: '100%', fontSize: 12.5, color: 'var(--text-2)', lineHeight: 1.5 }} html={data.history_note_html} />
          </div>
          <ul className="nums">
            {data.numbers.map(n => (
              <li key={n.id}>
                <span className="av">{n.initials}</span>
                <div><b>{n.label}</b><span className="no">{n.no}</span></div>
                <NumberStatus n={n} />
              </li>
            ))}
          </ul>
        </div>
        <div className="card">
          <div className="card-h"><h2>Batas &amp; privasi</h2><Icon n="i-lock" style={{ color: 'var(--text-3)' }} /><span className="meta">Sales melihat persis apa yang dibaca ARC</span></div>
          <ul className="rules">
            {data.rules.slice(0, split).map(ruleLi)}
            <li style={{ display: 'block' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}><div><b>Grup yang dibaca</b><span>ARC menggolongkan otomatis dari anggotanya; bisa diubah</span></div></div>
              <ul style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
                {data.groups.map(g => (
                  <li key={g.id} style={{ padding: 0, border: 0, display: 'flex', alignItems: 'center', gap: 8, fontSize: 12.5 }}>
                    <span className={'pill ' + (g.type === 'external' ? 'accent' : 'neutral')}>{g.type === 'external' ? 'eksternal' : 'internal'}</span>
                    {g.name}
                    {g.members_note && <span style={{ color: 'var(--text-3)', fontSize: 11.5 }}>· {g.members_note}</span>}
                    <Sw small on={g.read} onToggle={() => toggleGroup(g.id, !g.read)} label={g.name} />
                  </li>
                ))}
                {data.unlisted_groups > 0 && (
                  <li style={{ padding: 0, border: 0, fontSize: 12, color: 'var(--text-3)' }}>{data.unlisted_groups} grup lain tidak dibaca dan tidak terdaftar.</li>
                )}
              </ul>
            </li>
            {data.rules.slice(split).map(ruleLi)}
          </ul>
        </div>
      </div>
      <div className="stack">
        {p && <QrCard key={p.session_id + p.qr_png} p={p} onExpire={refresh} />}
        <div className="card">
          <div className="card-h"><h2>Yang diekstrak dari WhatsApp</h2><span className="meta" id="xt-period">{x.period}</span></div>
          <div className="xt">
            <div><b className="num">{fmtNum(x.messages)}</b><span>pesan dibaca</span></div>
            <div><b className="num">{fmtNum(x.commitments)}</b><span>komitmen terdeteksi</span></div>
            <div><b className="num">{fmtNum(x.contacts)}</b><span>kontak baru ditautkan</span></div>
            <div><b className="num">{fmtNum(x.sent)}</b><span>pesan dikirim ARC</span></div>
          </div>
        </div>
      </div>
    </div>
  )
}

function QrCard({ p, onExpire }: { p: NonNullable<SettingsWhatsApp['pairing']>; onExpire: () => void }) {
  const mutate = useMutate()
  const [left, setLeft] = useState(Math.max(0, Math.floor(p.expires_in)))
  useEffect(() => {
    const t = setInterval(() => setLeft(s => Math.max(0, s - 1)), 1000)
    return () => clearInterval(t)
  }, [])
  useEffect(() => {
    if (left === 0) onExpire()
  }, [left, onExpire])
  const mmss = Math.floor(left / 60) + ':' + String(left % 60).padStart(2, '0')
  return (
    <div className="card qr">
      <div className="card-h" style={{ alignSelf: 'stretch' }}><h2>Tautkan nomor {p.name}</h2><span className="meta">{p.branch}</span></div>
      <img src={p.qr_png} width={196} height={196} alt={'Kode QR untuk menautkan nomor ' + p.name}
        style={{ width: 196, height: 196, borderRadius: 14, background: '#fff', padding: 12, boxShadow: 'var(--shadow)' }} />
      <span className="exp">Berlaku <span id="qr-exp">{mmss}</span> · diperbarui otomatis</span>
      <ol>
        <li>Buka WhatsApp di ponsel {p.name}</li>
        <li>Ketuk ⋮ → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b></li>
        <li>Arahkan kamera ke kode ini</li>
        <li>ARC menarik riwayat <b id="qr-hist">{p.history_days} hari</b> dari ponsel, lalu membaca pesan baru tiap jam</li>
      </ol>
      <button className="btn ghost" style={{ alignSelf: 'stretch', justifyContent: 'center' }}
        onClick={() => mutate(api.post<Toast>('/api/wa/sessions/' + enc(p.session_id) + '/instructions'))}>
        Kirim petunjuk ke {p.name}
      </button>
    </div>
  )
}

// ---------- Nomor internal ----------
function Internal() {
  const { data, error } = useApi<SettingsInternal>('/api/internal-numbers')
  const mutate = useMutate()
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [unit, setUnit] = useState('')
  const [branch, setBranch] = useState('')
  if (!data) return <Loading error={error} />

  const unitV = unit || data.units[0] || ''
  const branchV = branch || data.branches[0] || ''
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const n = name.trim(), no = phone.trim()
    if (!n || !no) return
    const r = await mutate(api.post<Toast>('/api/internal-numbers', { name: n, phone: no, unit: unitV, branch: branchV }))
    if (r) { setName(''); setPhone(''); setUnit(''); setBranch('') }
  }

  return (
    <div className="int-grid conn-tab" id="sec-int">
      <div className="card">
        <div className="card-h"><h2>Nomor internal</h2><span className="meta" id="int-meta">{data.meta}</span></div>
        <p style={{ fontSize: 12.5, color: 'var(--text-2)', marginBottom: 12, lineHeight: 1.5 }}>
          Nomor yang terdaftar di sini dikenali sebagai <b>internal GSI</b>: tidak dihitung sebagai stakeholder pelanggan, tidak muncul di peta Network, pesan mereka di grup project dibaca sebagai koordinasi (tugas, jadwal, kendala), dan chat pribadinya tidak dibaca.
        </p>
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Nama</th><th>Nomor</th><th>Unit</th><th>Cabang</th><th>Sumber</th><th></th></tr></thead>
            <tbody id="int-rows">
              {data.rows.map(i => (
                <tr key={i.id}>
                  <td><b style={{ fontWeight: 600 }}>{i.n}</b></td>
                  <td className="mono">{i.no}</td>
                  <td><span className="pill neutral">{i.unit}</span></td>
                  <td>{i.branch}</td>
                  <td style={{ color: 'var(--text-3)', fontSize: 12 }}>{i.src}</td>
                  <td style={{ textAlign: 'right' }}>
                    <button className="btn quiet" style={{ height: 26, fontSize: 11.5 }} onClick={() => mutate(api.del<Toast>('/api/internal-numbers/' + i.id))}>Hapus</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
          <button className="btn ghost" onClick={() => mutate(api.post<Toast>('/api/internal-numbers/sync-talenta'))}><Icon n="i-refresh" />Sinkron dari Talenta</button>
          <span style={{ fontSize: 12, color: 'var(--text-3)', alignSelf: 'center' }}>Nomor karyawan diambil dari data HR; nomor bagian (gudang, admin) ditambah manual.</span>
        </div>
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Tambah nomor internal</h2></div>
          <form className="frm" id="int-form" onSubmit={submit}>
            <label>Nama<input id="int-name" placeholder="mis. Wawan Setiawan" required value={name} onChange={e => setName(e.target.value)} /></label>
            <label>Nomor WhatsApp<input id="int-no" placeholder="+62 8xx-xxxx-xxxx" required value={phone} onChange={e => setPhone(e.target.value)} /></label>
            <label>Unit<select id="int-unit" value={unitV} onChange={e => setUnit(e.target.value)}>{data.units.map(u => <option key={u}>{u}</option>)}</select></label>
            <label>Cabang<select id="int-branch" value={branchV} onChange={e => setBranch(e.target.value)}>{data.branches.map(b => <option key={b}>{b}</option>)}</select></label>
            <button type="submit" className="btn primary full" style={{ justifyContent: 'center' }}><Icon n="i-check" />Tambahkan sebagai internal</button>
          </form>
        </div>
        <div className="card">
          <div className="card-h"><h2>Dugaan ARC</h2><span className="ai" style={{ marginLeft: 6 }}>belum dikonfirmasi</span></div>
          <ul className="sus" id="int-sus">
            {data.suspects.length ? data.suspects.map(x => (
              <li key={x.id}>
                <div><b>{x.no} {x.n}</b><span>{x.why}</span></div>
                <div className="bt">
                  <button className="btn primary" onClick={() => mutate(api.post<Toast>('/api/internal-suspects/' + x.id + '/confirm'))}>Internal</button>
                  <button className="btn ghost" onClick={() => mutate(api.post<Toast>('/api/internal-suspects/' + x.id + '/reject'))}>Bukan</button>
                </div>
              </li>
            )) : <li><span>Tidak ada dugaan baru.</span></li>}
          </ul>
        </div>
      </div>
    </div>
  )
}

// ---------- AI & model ----------
function ToolPills({ t }: { t: McpTool }) {
  const kind = t.kind === 'read' ? <span className="pill neutral">read</span>
    : t.kind === 'write' ? <span className="pill accent">write</span>
      : t.kind === 'human-only' ? <span className="pill bad">human-only</span>
        : <span className="pill bad">CEO</span>
  return <>{kind}{t.approval && <span className="pill warn" style={{ marginLeft: 4 }}>approval</span>}</>
}

function AI() {
  const { data, error, setData } = useApi<SettingsAI>('/api/settings/ai')
  const mutate = useMutate()
  const copy = useCopy()
  if (!data) return <Loading error={error} />

  const toggleGroup = (id: string, enabled: boolean) => {
    setData({ ...data, mcp: { ...data.mcp, groups: data.mcp.groups.map(g => (g.id === id ? { ...g, enabled } : g)) } })
    mutate(api.post<Toast>('/api/settings/mcp-groups/' + enc(id), { enabled }))
  }
  const setRoute = (tier: string, option: string) => {
    setData({ ...data, routing: data.routing.map(r => (r.tier === tier ? { ...r, selected: option } : r)) })
    mutate(api.post<Toast>('/api/settings/routing', { tier, option }))
  }

  return (
    <div className="ai-grid conn-tab" id="sec-ai">
      <div className="card">
        <div className="card-h">
          <h2>ARC MCP Server</h2>
          {data.mcp.active
            ? <span className="pill good"><Icon n="i-check" />Aktif · v{data.mcp.version}</span>
            : <span className="pill neutral">Nonaktif · v{data.mcp.version}</span>}
          <span className="meta">Semua bagian ARC bisa diakses AI mana pun lewat satu endpoint</span>
        </div>
        <div className="ep">
          <Icon n="i-plug" style={{ color: 'var(--text-3)' }} />
          <span>{data.mcp.endpoint}</span>
          <button className="btn ghost" onClick={() => copy(data.mcp.endpoint)}>Salin</button>
        </div>
        <p style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 10, lineHeight: 1.5 }}>
          Autentikasi OAuth 2.1 per pengguna — Claude atau ChatGPT hanya melihat data yang boleh dilihat orang yang login. Tool yang menulis ke pelanggan selalu berhenti di antrean approval; tidak ada jalur langsung.
        </p>
        <div className="tools">
          {data.mcp.groups.map(g => (
            <div className="tg" key={g.id}>
              <div className="th">
                {g.label} <small>{g.tools.length} tool{g.note ? ' · ' + g.note : ''}</small>
                <Sw on={g.enabled} onToggle={() => toggleGroup(g.id, !g.enabled)} label={g.label} />
              </div>
              <ul>
                {g.tools.map(t => (
                  <li key={t.name}><code>{t.name}</code>{t.desc ? ' — ' + t.desc : ''}<ToolPills t={t} /></li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Klien AI terhubung</h2><span className="meta">Siapa yang boleh bertanya ke ARC</span></div>
          <div className="clients">
            {data.clients.map(c => (
              <div className="cl" key={c.id}>
                <span className="lg" style={{ background: c.color }}>{c.logo}</span>
                <div><b>{c.name}</b><span>{c.desc}</span></div>
                <div className="st">
                  <Pill p={c.status} />
                  {c.last && <small style={{ fontSize: 11, color: 'var(--text-3)' }}>{c.last}</small>}
                </div>
              </div>
            ))}
          </div>
          <div className="cfg" id="cfg-claude">
            <button className="cp" onClick={() => copy(data.config_snippet)}>Salin</button>
            {data.config_snippet}
          </div>
          <p style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 8, lineHeight: 1.5 }}>
            Claude.ai: Settings → Connectors → Add custom connector → tempel URL. ChatGPT: Settings → Connectors → tambah MCP server (Developer mode), atau Custom GPT dengan Actions dari{' '}
            <code style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5 }}>{data.openapi_url}</code>.
          </p>
        </div>
        <div className="card">
          <div className="card-h"><h2>Routing model</h2><span className="meta">Siapa menalar apa · biaya vs kualitas</span></div>
          <ul className="route">
            {data.routing.map(r => (
              <li key={r.tier}>
                <div><b>{r.title}</b><span>{r.sub}</span></div>
                <select value={r.selected} aria-label={r.title} onChange={e => setRoute(r.tier, e.target.value)}>
                  {r.options.map(o => <option key={o}>{o}</option>)}
                </select>
              </li>
            ))}
          </ul>
          <p style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 12, lineHeight: 1.5 }}>
            <b>Data tetap di server ARC</b> (self-hosted). Model hanya menerima potongan konteks yang dibutuhkan; nomor telepon dan rekening dimasking sebelum keluar ke provider eksternal. Estimasi biaya bulan ini: <b className="num">{data.cost_month}</b>.
          </p>
        </div>
      </div>
    </div>
  )
}

// ---------- MCP & API ----------
function ApiTab() {
  const { data, error } = useApi<SettingsAPI>('/api/settings/api')
  const mutate = useMutate()
  const copy = useCopy()
  const [newKey, setNewKey] = useState<string | null>(null)
  if (!data) return <Loading error={error} />

  const createKey = async () => {
    const r = await mutate(api.post<{ key: string; toast: string }>('/api/api-keys', {}))
    if (r?.key) setNewKey(r.key)
  }
  const pct = (rate: number) => Math.round(rate <= 1 ? rate * 100 : rate)

  return (
    <div className="ai-grid conn-tab" id="sec-api">
      <div className="card">
        <div className="card-h"><h2>ARC AI API</h2><span className="meta">REST + webhook · untuk agen atau sistem yang bukan MCP</span></div>
        <div className="ep">
          <span>{data.endpoint}</span>
          <button className="btn ghost" onClick={() => copy(data.endpoint)}>Salin</button>
          <button className="btn ghost" onClick={() => window.open('/openapi.json', '_blank', 'noopener')}>OpenAPI</button>
        </div>
        <div className="tbl-wrap" style={{ marginTop: 10 }}>
          <table className="api">
            <thead><tr><th>Endpoint</th><th>Fungsi</th></tr></thead>
            <tbody>
              {data.endpoints.map((e, i) => (
                <tr key={i}>
                  <td><span className={'m' + (e.method === 'POST' ? ' post' : '')}>{e.method}</span>{e.path && <> <code>{e.path}</code></>}</td>
                  <td><Html html={e.html} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="cfg" id="cfg-curl">
          <button className="cp" onClick={() => copy(data.curl)}>Salin</button>
          {data.curl}
        </div>
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>API key</h2><button className="btn primary" style={{ marginLeft: 'auto', height: 28, fontSize: 12 }} onClick={createKey}>+ Buat key</button></div>
          {newKey && (
            <div role="status" style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', padding: '10px 12px', marginBottom: 10, borderRadius: 12, background: 'var(--good-soft)', border: '1px solid var(--line)', fontSize: 12.5 }}>
              <Icon n="i-check" style={{ color: 'var(--good)' }} />
              <span style={{ flex: '1 1 160px', minWidth: 0 }}>
                <b style={{ display: 'block', fontWeight: 600 }}>Key baru · tampil sekali, simpan sekarang</b>
                <code style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5, wordBreak: 'break-all' }}>{newKey}</code>
              </span>
              <button className="btn ghost" style={{ height: 26, fontSize: 11.5 }} onClick={() => copy(newKey)}>Salin</button>
              <button className="btn quiet" style={{ height: 26, fontSize: 11.5 }} aria-label="Tutup" onClick={() => setNewKey(null)}><Icon n="i-x" /></button>
            </div>
          )}
          <ul className="keys" id="keys">
            {data.keys.map(k => (
              <li key={k.id}>
                <div><b>{k.name}</b><span>{k.line}</span></div>
                <span className={'pill ' + (k.active ? 'good' : 'neutral')}>{k.active ? 'aktif' : 'nonaktif'}</span>
              </li>
            ))}
          </ul>
        </div>
        <div className="card">
          <div className="card-h"><h2>Kalibrasi agen</h2><span className="ai" style={{ marginLeft: 6 }}>belajar dari penolakan</span><span className="meta">30 hari</span></div>
          <ul className="cal">
            {data.calibration.map(c => (
              <li key={c.agent}>
                <span>{c.agent}</span>
                <div className="bar"><i style={{ width: pct(c.rate) + '%', ...(c.warn ? { background: 'var(--warn)' } : {}) }} /></div>
                <span className="v num">{pct(c.rate)}%</span>
              </li>
            ))}
          </ul>
          <p style={{ fontSize: 11.5, color: 'var(--text-3)', margin: '4px 0 10px' }}>% saran yang disetujui. Di bawah 60% → agen dikalibrasi ulang.</p>
          <h3 style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)', marginBottom: 6 }}>Yang dipelajari dari penolakan</h3>
          <ul className="learn" id="learn">
            {data.learned.map((l, i) => (
              <li key={i}><span className="ai" /><Html html={l.html} /></li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  )
}
