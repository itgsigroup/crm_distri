import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MCPClient, MCPTool } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { useMCPClients, useMCPInfo, useMe } from '../../app/queries'
import { MCPClientEditor } from '../settings/AIConnections'
import { HermesChat, HermesChatConnect } from './HermesChat'
import { SCOPE } from '../claude/shared'

function copy(text: string, toast: (m: string) => void, secret = false) {
  if (!navigator.clipboard?.writeText) {
    toast(secret ? 'Clipboard tidak tersedia; salin dari blok secara manual' : text)
    return
  }
  void navigator.clipboard.writeText(text).then(() => toast('Disalin'), () => toast('Gagal menyalin; salin dari blok secara manual'))
}

function Code({ children, secret }: { children: string; secret?: boolean }) {
  const { toast } = useFeedback()
  return (
    <div className="cl-code">
      <pre>{children}</pre>
      <button className="btn ghost" onClick={() => copy(children, toast, secret)}><Icon name="doc" />Salin</button>
    </div>
  )
}

/** A connection made for Hermes: named after it (the token card suggests "Hermes Agent · <device>"). */
export const isHermes = (c: MCPClient) => c.kind === 'bearer' && /hermes/i.test(c.name)

export const hermesYAML = (endpoint: string, token: string) =>
  `mcp_servers:\n  distri_crm:\n    url: ${JSON.stringify(endpoint)}\n    headers:\n      Authorization: ${JSON.stringify(`Bearer ${token}`)}\n    connect_timeout: 60\n    timeout: 180`

const INSTALL = 'curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash'

/** Example tasks by role: what Hermes is good at with GSI Orbit's tools. */
const TASKS: { role: string; icon: string; items: [string, string][] }[] = [
  { role: 'CEO & manajer', icon: 'trend', items: [
    ['Ringkasan pagi', 'Pakai distri_crm: panggil data_ringkasan dan kpi_utama, lalu tulis ringkasan pagi 10 baris — cabang dan sales yang perlu perhatian hari ini, dan 3 tindakan prioritas.'],
    ['Tren cabang', 'Dengan penjualan_bulanan 12 bulan per cabang, cabang mana yang tumbuh dan mana yang turun? Jelaskan penyebabnya dari jadwal_lewat dan piutang_ringkas.'],
    ['Coaching sales', 'Dari dealer_list per sales, siapa yang paling banyak dealer lewat jadwal order? Beri saran coaching singkat untuk masing-masing.'],
  ] },
  { role: 'Admin & sales', icon: 'people', items: [
    ['Daftar follow-up', 'Panggil jadwal_lewat dan jadwal_due untuk cabang saya, urutkan dealer yang harus ditelepon hari ini beserta alasannya.'],
    ['Persiapan kunjungan', 'Ambil dealer_get untuk dealer <nama dealer>: ringkas pola order, sisa limit, produk yang biasa dibeli, dan produk yang bisa ditawarkan.'],
    ['Push stok', 'Dari stok_aging (min_days 90) dan produk_terlaris 90 hari, susun daftar barang yang perlu didorong dan dealer kandidatnya.'],
  ] },
  { role: 'Keuangan', icon: 'cash', items: [
    ['Risiko piutang', 'Analisis piutang_ringkas: dealer mana yang berisiko macet, berapa prediksi kas 30 hari, dan dealer mana yang limitnya perlu ditinjau.'],
    ['Cek kredit', 'Pakai kredit_check untuk dealer <nama dealer> dengan order Rp 25.000.000 — apakah masih dalam limit dan apa catatannya?'],
    ['Analisis kas', 'Jalankan analisis_kas dan jelaskan hasilnya dalam bahasa sederhana untuk rapat keuangan minggu ini.'],
  ] },
]

const RULES: [string, string, string][] = [
  ['lock', 'Tidak pernah memutuskan atau mengirim', 'Hermes hanya membaca dan mengusulkan. Setujui/tolak, kirim pesan ke dealer, ubah harga atau limit tetap dilakukan manusia di GSI Orbit.'],
  ['shield', 'Nomor & email disamarkan', 'Nomor WhatsApp, telepon, dan email disamarkan sebelum sampai ke Hermes (atur di MCP Claude → Izin & batas).'],
  ['qr', 'Token = kunci rumah', 'Token Hermes memberi akses baca ke data perusahaan. Simpan hanya di device Hermes, jangan dibagikan di chat atau email, dan cabut bila device hilang.'],
  ['refresh', 'Rotasi berkala', 'Ganti token tiap beberapa bulan atau saat ada pergantian karyawan: Edit → Rotasi token. Token lama langsung mati.'],
  ['chart', 'Semua panggilan tercatat', 'Setiap tool yang dipanggil Hermes tercatat (siapa, kapan, tool apa) dan bisa dilihat di MCP Claude → Panggilan.'],
  ['alert', 'Cek ulang angka penting', 'Angka orbit dihitung oleh sistem, tetapi kesimpulan Hermes tetap tulisan AI. Untuk keputusan besar, buka data aslinya di GSI Orbit.'],
]

const FAQ: [string, string][] = [
  ['Apa bedanya Hermes dengan Claude di halaman MCP Claude?', 'Keduanya klien MCP yang memakai tool GSI Orbit yang sama. Claude (claude.ai / Claude Code) login lewat OAuth dari akun Anda. Hermes Agent adalah agent open-source dari Nous Research yang berjalan di laptop, mini PC, atau server Anda sendiri, terhubung dengan token manual, dan bisa memakai model AI pilihan Anda.'],
  ['Apakah Hermes perlu dipasang di server CRM?', 'Tidak. Hermes berjalan di device lain dan memanggil GSI Orbit lewat HTTPS. Server CRM tidak perlu diubah.'],
  ['"hermes mcp test distri_crm" gagal dengan 401', 'Token salah, sudah dirotasi, atau koneksinya dicabut. Buat token baru (atau Edit → Rotasi token) lalu ganti blok YAML di ~/.hermes/config.yaml dan restart Hermes.'],
  ['Hermes timeout saat memanggil tool', 'Tool analisis bisa butuh waktu lebih lama. Pastikan timeout: 180 ada di konfigurasi, cek koneksi internet device Hermes, dan coba lagi dengan permintaan yang lebih sempit (satu cabang atau satu dealer).'],
  ['Hermes tidak menemukan tool distri_crm', 'Pastikan blok YAML berada di bawah mcp_servers (bukan di luar), indentasi memakai spasi, lalu restart Hermes. Jalankan hermes mcp test distri_crm untuk memastikan.'],
  ['Bisakah Hermes mengirim WhatsApp ke dealer?', 'Tidak. Klien MCP tidak pernah bisa mengirim atau memutuskan. Hermes bisa menyiapkan draf atau usulan; manusia yang menyetujui dan mengirim dari GSI Orbit.'],
  ['Izin apa yang sebaiknya dipilih?', 'Mulai dengan Baca + Analisis. Izin Orchestrator hanya bila Hermes memang perlu memicu siklus analisis ulang, dan tetap dibatasi jumlah siklus per jam.'],
]

const TERMS: [string, string][] = [
  ['MCP', 'Model Context Protocol — "colokan" standar agar agent AI bisa memakai tool sebuah aplikasi. GSI Orbit menyediakan server MCP; Hermes menjadi kliennya.'],
  ['Tool', 'Satu kemampuan yang bisa dipanggil agent, misalnya dealer_list atau piutang_ringkas. Agent memilih tool sendiri sesuai pertanyaan.'],
  ['Token', 'Kata sandi khusus untuk mesin. Ditampilkan sekali saat dibuat; yang tersimpan di CRM hanya awalannya.'],
  ['Izin (scope)', 'Batas apa yang boleh dilakukan token: Baca, Analisis, Orchestrator. Keputusan tidak pernah termasuk.'],
  ['Provenance', 'Setiap usulan AI membawa sumber datanya (sinyal, waktu, confidence) sehingga bisa diperiksa ulang.'],
]

function Hero({ clients }: { clients: MCPClient[] }) {
  const { data: info } = useMCPInfo()
  const { toast } = useFeedback()
  const active = clients.filter((c) => c.active)
  const last = active.map((c) => c.last_seen_at).filter(Boolean).sort().pop()
  return (
    <div className="card cl-hero">
      <span className="cl-logo hm-logo"><Icon name="spark" /></span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <h2>Hermes Agent untuk GSI Orbit</h2>
        <p>Hermes Agent adalah asisten AI yang berjalan di device Anda sendiri. Setelah disambungkan, Hermes bisa membaca dealer, penjualan, piutang, stok, dan chat (nomor disamarkan), lalu merangkum dan menganalisis untuk Anda. Hermes <b>mengusulkan</b>; keputusan dan pengiriman ke dealer tetap di tangan manusia.</p>
        <div className="cl-status">
          <Pill tone={info?.enabled ? 'good' : 'bad'} icon={info?.enabled ? 'check' : 'alert'}>{info?.enabled ? 'Server MCP aktif' : 'Server MCP mati'}</Pill>
          <Pill tone={active.length ? 'accent' : 'neutral'} icon="plug">{active.length} koneksi Hermes aktif</Pill>
          <Pill tone="neutral">{last ? `Terakhir dipakai ${shortDate(last)} · ${hhmm(last)}` : 'Belum pernah dipakai'}</Pill>
        </div>
        {info?.endpoint && <div className="ep" style={{ marginTop: 10 }}><span>{info.endpoint}</span><button className="btn ghost" onClick={() => copy(info.endpoint, toast)}>Salin URL</button></div>}
      </div>
    </div>
  )
}

function HowItWorks() {
  return (
    <div className="card">
      <div className="card-h"><h2>Cara kerjanya</h2><span className="meta">3 langkah, sekali jalan</span></div>
      <div className="hm-flow" aria-label="Alur Hermes ke GSI Orbit">
        <div><Icon name="people" /><b>Anda bertanya</b><span>Ketik pertanyaan ke Hermes di tab Chat halaman ini, Telegram, atau terminal</span></div>
        <i aria-hidden="true">→</i>
        <div><Icon name="spark" /><b>Hermes memanggil tool</b><span>Lewat HTTPS + token ke server MCP GSI Orbit, sesuai izin token</span></div>
        <i aria-hidden="true">→</i>
        <div><Icon name="doc" /><b>Jawaban & usulan</b><span>Hermes merangkum data; usulan tindakan tetap diputuskan di aplikasi</span></div>
      </div>
    </div>
  )
}

function Connect({ endpoint, clients }: { endpoint: string; clients: MCPClient[] }) {
  const { data: me } = useMe()
  const qc = useQueryClient()
  const { toast, openSheet } = useFeedback()
  const [device, setDevice] = useState('')
  const [scopes, setScopes] = useState<string[]>(['read', 'analyze'])
  const [token, setToken] = useState('')
  const create = useMutation({
    mutationFn: () => api.post<{ token: string }>('/mcp/clients', { name: `Hermes Agent · ${device.trim()}`, scopes }),
    onSuccess: (r) => { setToken(r.token); setDevice(''); qc.invalidateQueries({ queryKey: ['mcp'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/mcp/clients/${id}`),
    onSuccess: () => { toast('Koneksi Hermes dicabut'); qc.invalidateQueries({ queryKey: ['mcp'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const edit = !!me?.edit_policies
  const flip = (s: string) => setScopes(scopes.includes(s) ? scopes.filter((x) => x !== s) : [...scopes, s])
  return (
    <div className="card">
      <div className="card-h"><h2>Koneksi Hermes</h2><span className="meta">{edit ? 'Satu token per device' : 'Token dibuat oleh pemegang hak kebijakan'}</span></div>
      {edit && (
        <>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <input className="cl-input" value={device} onChange={(e) => setDevice(e.target.value)} placeholder="Nama device, mis. Mini PC kantor" aria-label="Nama device Hermes" />
            <button className="btn primary" disabled={!device.trim() || !scopes.length || create.isPending} onClick={() => create.mutate()}><Icon name="plug" />Buat token Hermes</button>
          </div>
          <div className="chips" style={{ marginTop: 10 }}>
            {['read', 'analyze', 'orchestrate'].map((s) => <button key={s} className={`chip ${scopes.includes(s) ? 'is-active' : ''}`} title={SCOPE[s][1]} onClick={() => flip(s)}>{SCOPE[s][0]}</button>)}
          </div>
        </>
      )}
      {token && (
        <div className="cl-token">
          <b>Konfigurasi Hermes (ditampilkan sekali — salin sekarang)</b>
          <span>Tambahkan ke <code>~/.hermes/config.yaml</code> di bawah <code>mcp_servers</code>:</span>
          <Code secret>{hermesYAML(endpoint, token)}</Code>
        </div>
      )}
      <ul className="hm-conns">
        {clients.length === 0 && <li className="cl-empty">Belum ada koneksi Hermes. {edit ? 'Buat token di atas, lalu ikuti langkah pemasangan.' : 'Minta CEO membuat token Hermes.'}</li>}
        {clients.map((c) => (
          <li key={c.id} className={c.active ? undefined : 'is-off'}>
            <div><b>{c.name}</b><small>{c.scopes.map((s) => SCOPE[s]?.[0] ?? s).join(', ')} · {c.last_seen_at ? `terakhir ${shortDate(c.last_seen_at)} ${hhmm(c.last_seen_at)}` : 'belum dipakai'} · {c.calls_today.toLocaleString('id-ID')} panggilan hari ini</small></div>
            <Pill tone={c.active ? 'good' : 'neutral'}>{c.active ? c.token_prefix || 'Aktif' : 'Dicabut'}</Pill>
            {c.active && edit && <>
              <button className="btn ghost" onClick={() => openSheet(<MCPClientEditor client={c} />)}>Edit</button>
              <button className="btn ghost" onClick={() => { if (window.confirm(`Cabut koneksi ${c.name}? Hermes di device itu langsung tidak bisa membaca data.`)) revoke.mutate(c.id) }}>Cabut</button>
            </>}
          </li>
        ))}
      </ul>
    </div>
  )
}

function Setup({ endpoint }: { endpoint: string }) {
  return (
    <div className="card">
      <div className="card-h"><h2>Langkah pemasangan</h2><span className="meta">± 10 menit · di device Hermes</span></div>
      <ol className="cl-steps">
        <li><b>Pasang Hermes Agent</b> di laptop, mini PC, atau server (macOS, Linux, atau Windows lewat WSL2). Panduan resmi: <a href="https://github.com/NousResearch/hermes-agent" target="_blank" rel="noreferrer">github.com/NousResearch/hermes-agent</a>.
          <Code>{INSTALL}</Code>
          Lalu jalankan <code>hermes setup</code> untuk memilih model AI yang dipakai Hermes.</li>
        <li><b>Buat token</b> di kartu <b>Koneksi Hermes</b> — satu token per device, beri nama device-nya. Izin <code>Baca</code> + <code>Analisis</code> cukup untuk hampir semua kebutuhan.</li>
        <li><b>Tempel konfigurasi</b> yang muncul ke <code>~/.hermes/config.yaml</code> di bawah <code>mcp_servers</code>. Jika sudah ada server MCP lain, cukup tambahkan blok <code>distri_crm</code>. Server: <code>{endpoint || 'https://…/mcp'}</code>
          <Code>{'nano ~/.hermes/config.yaml\nchmod 600 ~/.hermes/config.yaml'}</Code></li>
        <li><b>Uji koneksi</b> — restart Hermes lalu jalankan:
          <Code>hermes mcp test distri_crm</Code>
          Hasil yang benar menampilkan daftar tool GSI Orbit.</li>
        <li><b>Mulai bertanya</b> — misalnya “Pakai distri_crm, ringkas dealer yang perlu ditindaklanjuti hari ini.” Contoh lain ada di kartu <b>Contoh tugas</b>.</li>
      </ol>
    </div>
  )
}

function Tasks() {
  const [role, setRole] = useState(0)
  const { toast } = useFeedback()
  return (
    <div className="card">
      <div className="card-h"><h2>Contoh tugas</h2><span className="meta">Salin ke Hermes</span></div>
      <div className="chips" role="tablist" style={{ marginBottom: 12 }}>
        {TASKS.map((t, i) => <button key={t.role} role="tab" aria-selected={role === i} className={`chip ${role === i ? 'is-active' : ''}`} onClick={() => setRole(i)}><Icon name={t.icon} />{t.role}</button>)}
      </div>
      <ul className="hm-tasks">
        {TASKS[role].items.map(([title, prompt]) => (
          <li key={title}>
            <div><b>{title}</b><span>{prompt}</span></div>
            <button className="btn quiet" onClick={() => copy(prompt, toast)}><Icon name="doc" />Salin</button>
          </li>
        ))}
      </ul>
    </div>
  )
}

function Rules() {
  return (
    <div className="card">
      <div className="card-h"><h2>Aturan main & keamanan</h2><span className="meta">Wajib dibaca pengguna Hermes</span></div>
      <ul className="hm-rules">
        {RULES.map(([icon, title, text]) => <li key={title}><Icon name={icon} /><div><b>{title}</b><span>{text}</span></div></li>)}
      </ul>
    </div>
  )
}

const GROUPS = ['read', 'analyze', 'orchestrate', 'decide']

function Tools() {
  const { data: info } = useMCPInfo()
  const tools = info?.tools ?? []
  const by = (g: string) => tools.filter((t: MCPTool) => t.scope === g)
  return (
    <div className="card cl-tools">
      <div className="card-h"><h2>Tool yang bisa dipakai Hermes</h2><span className="meta">{tools.length} tool · Hermes memilih sendiri</span></div>
      {GROUPS.filter((g) => by(g).length).map((g) => (
        <div key={g}>
          <h4>{SCOPE[g]?.[0] ?? g} <span>· {SCOPE[g]?.[1]}</span></h4>
          <ul>{by(g).map((t) => <li key={t.name}><code>{t.name}</code><span>{t.description}</span></li>)}</ul>
        </div>
      ))}
    </div>
  )
}

function Faq() {
  return (
    <div className="card">
      <div className="card-h"><h2>Tanya-jawab & kendala</h2></div>
      <div className="hm-faq">
        {FAQ.map(([q, a]) => <details key={q}><summary>{q}</summary><p>{a}</p></details>)}
      </div>
    </div>
  )
}

function Terms() {
  return (
    <div className="card">
      <div className="card-h"><h2>Istilah</h2></div>
      <dl className="hm-terms">
        {TERMS.map(([t, d]) => <div key={t}><dt>{t}</dt><dd>{d}</dd></div>)}
      </dl>
    </div>
  )
}

const TABS = [['chat', 'Chat'], ['setup', 'Panduan & koneksi']] as const
type Tab = (typeof TABS)[number][0]

/** Sidebar → Hermes Agent: chat with Hermes like in Telegram, and what Hermes is, connecting it to GSI Orbit's MCP
 * server, and how to use it safely. */
export function HermesPage() {
  const { data: info } = useMCPInfo()
  const { data: all = [] } = useMCPClients()
  const clients = all.filter(isHermes)
  const [tab, setTab] = useState<Tab>(() => (window.location.hash === '#panduan' ? 'setup' : 'chat'))
  const go = (t: Tab) => { setTab(t); window.history.replaceState(null, '', t === 'setup' ? '#panduan' : window.location.pathname) }
  return (
    <div className="stack">
      <div className="seg hc-tabs" role="tablist" aria-label="Hermes Agent">
        {TABS.map(([k, label]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'is-active' : ''} onClick={() => go(k)}>{label}</button>)}
      </div>
      {tab === 'chat' ? <HermesChat onSettings={() => go('setup')} /> : <>
        <Hero clients={clients} />
        <HermesChatConnect />
        <HowItWorks />
        <div className="ai-grid">
          <div className="stack">
            <Connect endpoint={info?.endpoint ?? ''} clients={clients} />
            <Setup endpoint={info?.endpoint ?? ''} />
            <Tasks />
          </div>
          <div className="stack">
            <Rules />
            <Faq />
            <Terms />
          </div>
        </div>
        <Tools />
      </>}
    </div>
  )
}
