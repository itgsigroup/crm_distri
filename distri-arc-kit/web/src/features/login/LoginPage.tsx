import { useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router'

/** Login (the mockup has no login screen: a brand card in the mockup's tokens). */
export function LoginPage() {
  const [params] = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    const r = await fetch('/api/auth/login', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }) })
    setBusy(false)
    if (!r.ok) {
      const d = await r.json().catch(() => null)
      setError(d?.error?.message ?? 'Login gagal')
      return
    }
    const next = params.get('next')
    window.location.assign(next && next.startsWith('/') && !next.startsWith('//') && next !== '/login' ? next : '/')
  }
  const field = { width: '100%', height: 40, borderRadius: 10, border: '1px solid var(--line)', padding: '0 12px', font: 'inherit', fontSize: 14, background: 'var(--surface)', color: 'var(--text)', marginTop: 4 }
  return (
    <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', background: 'var(--bg)', padding: 16 }}>
      <form className="card" onSubmit={submit} style={{ width: '100%', maxWidth: 380, display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div className="brand" style={{ marginBottom: 6 }}>
          <div className="brand-mark" />
          <div><div className="brand-name">Distri ARC</div><div className="brand-sub">Orbit · AI-native</div></div>
        </div>
        <label style={{ fontSize: 12, color: 'var(--text-2)' }}>Email<input style={field} type="email" name="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required /></label>
        <label style={{ fontSize: 12, color: 'var(--text-2)' }}>Kata sandi<input style={field} type="password" name="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required /></label>
        {error && <div className="pill bad" style={{ alignSelf: 'flex-start' }}>{error}</div>}
        <button className="btn primary" type="submit" disabled={busy} style={{ justifyContent: 'center', height: 40 }}>{busy ? 'Masuk…' : 'Masuk'}</button>
        <span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>Akun dibuat oleh CEO / admin di Pengaturan → Pengguna &amp; peran.</span>
      </form>
    </div>
  )
}
