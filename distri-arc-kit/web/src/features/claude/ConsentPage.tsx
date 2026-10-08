import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'

interface OAuthRequest {
  id: string; client_name: string; redirect_host: string; allowed: boolean; reason: string
  scopes: { key: string; label: string; requested: boolean; allowed: boolean }[]
  user: { name: string; email: string; role: string }
}

/** /claude/izin: a person approves (or not) a Claude connection; the browser then returns to Claude. */
export function ConsentPage() {
  const [params] = useSearchParams()
  const id = params.get('req') ?? ''
  const { data, error } = useQuery({ queryKey: ['oauth', id], enabled: !!id, retry: false, queryFn: () => api.get<OAuthRequest>(`/oauth/requests/${encodeURIComponent(id)}`) })
  const [picked, setPicked] = useState<string[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const chosen = picked ?? (data?.scopes.filter((s) => s.requested && s.allowed).map((s) => s.key) ?? [])
  const decide = (approve: boolean) => {
    setBusy(true)
    api.post<{ redirect: string }>(`/oauth/requests/${encodeURIComponent(id)}/${approve ? 'approve' : 'deny'}`, { scopes: chosen })
      .then((r) => window.location.assign(r.redirect), (e: Error) => { setErr(e.message); setBusy(false) })
  }
  return (
    <div className="consent">
      <div className="card">
        <div className="consent-h">
          <span className="cl-logo"><Icon name="spark" /></span>
          <div><h2>{data ? <><b>{data.client_name}</b> ingin terhubung ke GSI Orbit</> : 'Izinkan Claude'}</h2>
            <p>{data ? <>Kembali ke <b>{data.redirect_host}</b> setelah Anda memutuskan · masuk sebagai {data.user.name} ({data.user.role})</> : ''}</p></div>
        </div>
        {!id || error ? (
          <p className="consent-bad">{error ? (error as Error).message : 'Permintaan koneksi tidak ditemukan.'} Mulai lagi dari Claude (Connect / Authenticate).</p>
        ) : !data ? <p className="cl-empty">Memuat…</p> : (
          <>
            <h4 className="consent-sub">Claude akan bisa</h4>
            <ul className="consent-scopes">
              {data.scopes.map((s) => (
                <li key={s.key} className={s.allowed ? '' : 'off'}>
                  <input type="checkbox" checked={chosen.includes(s.key)} disabled={!s.allowed || s.key === 'read' || !data.allowed} onChange={() => setPicked(chosen.includes(s.key) ? chosen.filter((x) => x !== s.key) : [...chosen, s.key])} aria-label={s.key} />
                  <div><b>{s.key === 'read' ? 'Membaca data' : s.key === 'analyze' ? 'Menjalankan analisis' : 'Menjalankan Orchestrator'}</b><span>{s.label}{!s.allowed ? ' — tidak tersedia untuk peran Anda' : ''}</span></div>
                </li>
              ))}
              <li className="off"><input type="checkbox" checked={false} disabled aria-label="decide" /><div><b>Memutuskan atau mengirim ke dealer</b><span>Tidak pernah — setujui/kirim hanya di aplikasi oleh manusia</span></div></li>
            </ul>
            {!data.allowed && <p className="consent-bad">{data.reason}</p>}
            {err && <p className="consent-bad">{err}</p>}
            <div className="consent-ft">
              {data.allowed && <button className="btn primary" disabled={busy} onClick={() => decide(true)}><Icon name="check" />Izinkan</button>}
              <button className="btn ghost" disabled={busy} onClick={() => decide(false)}>Tolak</button>
              <span className="pol"><Icon name="lock" />Koneksi bisa dicabut kapan saja di menu MCP Claude</span>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
