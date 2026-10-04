import { useState, type FormEvent } from 'react'
import { api } from '../api/client'
import type { Me } from '../api/types'
import { useUI } from '../state/ui'

export function Login() {
  const { setMe } = useUI()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      setMe(await api.post<Me>('/api/auth/login', { email, password }))
    } catch (x) {
      setErr(x instanceof Error ? x.message : String(x))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="login">
      <form className="card" onSubmit={submit}>
        <div className="brand">
          <div className="brand-mark" />
          <div><div className="brand-name">ARC</div><div className="brand-sub">Relationship Core</div></div>
        </div>
        <label>Email<input type="email" value={email} onChange={e => setEmail(e.target.value)} autoComplete="username" required /></label>
        <label>Kata sandi<input type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" required /></label>
        {err && <div className="err">{err}</div>}
        <button className="btn primary" style={{ justifyContent: 'center' }} disabled={busy}>Masuk</button>
        <p className="hint">Pengguna internal GSI. Akses mengikuti peran dan cabang Anda.</p>
      </form>
    </div>
  )
}
