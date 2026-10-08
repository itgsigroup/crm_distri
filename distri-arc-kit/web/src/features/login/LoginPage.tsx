import { PasswordInput } from '../../components/PasswordInput'
import { Icon } from '../../components/Icon'
import sprite from '../../icons/sprite.svg?raw'
import { useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router'

// The login is a one-page landing: what Distri ARC does (features, how it works, security) next to the sign-in card.

const FEATURES: [string, string, string][] = [
  ['spark', 'Orchestrator & 6 agen AI', 'AI Order, Follow-up, Kredit, Penagihan, Stok, dan Prospek bekerja tiap jam dalam satu siklus tercatat — hasilnya usulan, bukan aksi.'],
  ['radar', 'Orbit & Segmen', 'Setiap dealer dipetakan menurut siklus order, skor kesehatan, dan segmen — yang mulai menjauh terlihat sebelum hilang.'],
  ['sun', 'Pusat kendali', 'Rencana hari ini, antrean keputusan, KPI order tepat jadwal, DSO, dan perputaran stok dalam satu layar.'],
  ['chat', 'Chat WhatsApp multi-nomor', 'Satu pengguna satu nomor, ditautkan lewat scan QR; percakapan dealer dibaca agen untuk menangkap order dan janji bayar.'],
  ['box', 'Push stok', 'Stok menua per cabang dipasangkan dengan dealer kandidat yang paling mungkin membeli, lengkap dengan draft penawaran.'],
  ['cash', 'Kredit · kas', 'Sisa limit, piutang lewat tempo, prediksi kas masuk, dan dealer yang limitnya perlu ditinjau.'],
  ['plug', 'MCP Claude & analisis terjadwal', 'Claude menganalisis semua data lewat MCP dan menulis laporan sesuai jadwal cron — tanpa pernah mengirim apa pun.'],
  ['refresh', 'Data Accurate tersinkron', 'Pelanggan, faktur, item, dan stok gudang ditarik dari BigQuery secara berkala; angka dihitung dengan rumus yang sama untuk semua.'],
  ['shield', 'Peran & akses kustom', 'Peran, halaman yang dibuka, cakupan data, dan hak memutuskan diatur sendiri per cabang dan per pengguna.'],
]

const STEPS: [string, string][] = [
  ['Data masuk', 'Faktur, pembayaran, stok, dan chat WhatsApp terkumpul otomatis dari Accurate dan nomor tim.'],
  ['AI mengusulkan', 'Orchestrator menjalankan agen, menghitung angka di sistem, dan menyusun usulan beserta sumbernya.'],
  ['Manusia memutuskan', 'Sales, admin, atau CEO menyetujui, mengubah, atau menolak — baru kemudian sesuatu sampai ke dealer.'],
]

const YEAR = new Date().getFullYear()

const SECURITY = ['Keputusan selalu oleh manusia', 'Nomor & email disamarkan ke AI', 'Login 2 langkah (2FA)', 'Jejak audit setiap aksi', 'Akses per peran & cabang']

/** Login: a one-page landing (features of the system) with the sign-in card. */
export function LoginPage() {
  const [params] = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    const r = await fetch('/api/auth/login', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password, code }) })
    setBusy(false)
    if (!r.ok) {
      const d = await r.json().catch(() => null)
      if (d?.error?.code === 'totp_required') {
        setNeedCode(true) // second step: the authenticator code
        setError('')
        return
      }
      setError(d?.error?.message ?? 'Login gagal')
      return
    }
    const next = params.get('next')
    window.location.assign(next && next.startsWith('/') && !next.startsWith('//') && next !== '/login' ? next : '/')
  }
  const toLogin = () => {
    document.getElementById('masuk')?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    window.setTimeout(() => (document.querySelector('.lp-card input[name=email]') as HTMLInputElement | null)?.focus(), 350)
  }
  return (
    <div className="lp">
      <div style={{ position: 'absolute', width: 0, height: 0, overflow: 'hidden' }} dangerouslySetInnerHTML={{ __html: sprite }} />
      <header className="lp-nav">
        <div className="lp-brand"><div className="brand-mark" /><div><b>Distri ARC</b><span>Orbit · AI-native</span></div></div>
        <nav>
          <a href="#fitur">Fitur</a>
          <a href="#cara-kerja">Cara kerja</a>
          <a href="#keamanan">Keamanan</a>
        </nav>
        <button type="button" className="btn primary" onClick={toLogin}>Masuk</button>
      </header>

      <section className="lp-hero">
        <div className="lp-hero-txt">
          <span className="lp-eyebrow"><span className="lp-dot" />CRM distribusi B2B · PT Gosyen Solusi Indonesia</span>
          <h1>Jaga setiap dealer tetap <em>di orbit.</em></h1>
          <p>Distri ARC membaca order, piutang, stok, dan chat WhatsApp setiap jam, lalu menyiapkan langkah berikutnya untuk tim sales — siapa yang perlu dihubungi, apa yang ditawarkan, dan kapan menagih. AI mengusulkan, Anda yang memutuskan.</p>
          <ul className="lp-stats">
            <li><b>6</b><span>agen AI dalam satu Orchestrator</span></li>
            <li><b>Tiap jam</b><span>siklus analisis 06.00–20.00</span></li>
            <li><b>100%</b><span>keputusan oleh manusia</span></li>
          </ul>
        </div>
        <div className="lp-side" id="masuk">
          <div className="lp-orbit" aria-hidden="true">
            <span className="r r1"><i /></span><span className="r r2"><i /><i /></span><span className="r r3"><i /></span><span className="core" />
          </div>
          <form className="card lp-card" onSubmit={submit}>
            <h2>Masuk</h2>
            <p className="lp-card-sub">Gunakan akun Distri ARC dari admin Anda.</p>
            <label>Email<input type="email" name="email" autoComplete="username" placeholder="nama@gsicctv.com" value={email} onChange={(e) => setEmail(e.target.value)} required /></label>
            <label>Kata sandi<PasswordInput name="password" autoComplete="current-password" placeholder="Kata sandi" value={password} onChange={(e) => setPassword(e.target.value)} required /></label>
            {needCode && <label>Kode 2FA<input className="lp-code" name="code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} autoFocus value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} required /></label>}
            {error && <div className="pill bad" style={{ alignSelf: 'flex-start' }}>{error}</div>}
            <button className="btn primary lp-submit" type="submit" disabled={busy}>{busy ? 'Masuk…' : 'Masuk'}</button>
            <span className="lp-card-note"><Icon name="lock" />Akun dibuat lewat Master data → Pengguna. Lupa kata sandi? Hubungi admin.</span>
          </form>
        </div>
      </section>

      <section className="lp-sec" id="fitur">
        <h2>Semua yang dibutuhkan tim distribusi</h2>
        <p className="lp-lead">Dari dealer yang mulai jarang order sampai stok yang menua di gudang — satu sistem, satu sumber angka.</p>
        <div className="lp-grid">
          {FEATURES.map(([icon, title, text]) => (
            <article key={title} className="lp-feat">
              <span className="lp-ic"><Icon name={icon} /></span>
              <h3>{title}</h3>
              <p>{text}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="lp-sec" id="cara-kerja">
        <h2>Cara kerja</h2>
        <ol className="lp-steps">
          {STEPS.map(([t, d], i) => <li key={t}><span>{i + 1}</span><div><b>{t}</b><p>{d}</p></div></li>)}
        </ol>
      </section>

      <section className="lp-sec lp-sec-last" id="keamanan">
        <h2>Aman dan bisa diaudit</h2>
        <ul className="lp-sec-list">{SECURITY.map((s) => <li key={s}><Icon name="check" />{s}</li>)}</ul>
      </section>

      <footer className="lp-foot">
        <span>© {YEAR} PT Gosyen Solusi Indonesia · Distri ARC Orbit</span>
        <button type="button" className="btn ghost" onClick={toLogin}>Masuk ke sistem</button>
      </footer>
    </div>
  )
}
