import { PasswordInput } from '../../components/PasswordInput'
import { Icon } from '../../components/Icon'
import sprite from '../../icons/sprite.svg?raw'
import { useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router'
import { Galaxy } from './Galaxy'

// The login is one screen (no scrolling): what Distri ARC does on the left, the sign-in card on the right.

const FEATURES: [string, string, string][] = [
  ['spark', 'Orchestrator & 6 agen AI', 'Order, follow-up, kredit, tagih, stok, prospek'],
  ['radar', 'Orbit & Segmen', 'Dealer yang mulai menjauh terlihat lebih awal'],
  ['sun', 'Pusat kendali', 'Rencana hari ini, keputusan, dan KPI'],
  ['chat', 'Chat WhatsApp', 'Satu pengguna satu nomor, scan QR'],
  ['box', 'Push stok', 'Stok menua dipasangkan dengan dealer kandidat'],
  ['cash', 'Kredit · kas', 'Sisa limit, piutang, prediksi kas masuk'],
  ['plug', 'MCP Claude', 'Analisis semua data, laporan terjadwal'],
  ['refresh', 'Data Accurate', 'Faktur, pelanggan, stok tersinkron'],
  ['shield', 'Peran & akses', 'Halaman, cabang, dan hak memutuskan'],
]

const YEAR = new Date().getFullYear()

/** Login: one screen with the system's features and the sign-in card. */
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
  return (
    <div className="lp">
      <div style={{ position: 'absolute', width: 0, height: 0, overflow: 'hidden' }} dangerouslySetInnerHTML={{ __html: sprite }} />
      <Galaxy />
      <header className="lp-nav">
        <div className="lp-brand"><div className="brand-mark" /><div><b>Distri ARC</b><span>Orbit · AI-native</span></div></div>
        <span className="lp-org">PT Gosyen Solusi Indonesia</span>
      </header>

      <main className="lp-main">
        <section className="lp-info">
          <span className="lp-eyebrow"><span className="lp-dot" />CRM distribusi B2B</span>
          <h1>Jaga setiap dealer tetap <em>di orbit.</em></h1>
          <p className="lp-lead">Order, piutang, stok, dan chat WhatsApp dibaca setiap jam; tim sales mendapat langkah berikutnya. AI mengusulkan, Anda yang memutuskan.</p>
          <ul className="lp-feats">
            {FEATURES.map(([icon, title, text]) => (
              <li key={title}>
                <span className="lp-ic"><Icon name={icon} /></span>
                <div><b>{title}</b><span>{text}</span></div>
              </li>
            ))}
          </ul>
        </section>

        <section className="lp-side">
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
        </section>
      </main>

      <footer className="lp-foot">
        <span>© {YEAR} PT Gosyen Solusi Indonesia</span>
        <span className="lp-trust"><Icon name="check" />Keputusan oleh manusia<Icon name="check" />PII disamarkan ke AI<Icon name="check" />2FA &amp; jejak audit</span>
      </footer>
    </div>
  )
}
