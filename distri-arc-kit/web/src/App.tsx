import { useEffect, useState } from 'react'

type Health = { db: string; queue: string }

// Stage 00 placeholder: brand, tokens and fonts from the mockup, plus the API health.
export default function App() {
  const [health, setHealth] = useState<Health | null>(null)
  useEffect(() => {
    fetch('/api/health').then((r) => r.json()).then(setHealth).catch(() => setHealth(null))
  }, [])
  return (
    <main className="stage" style={{ padding: 32 }}>
      <div className="brand">
        <div className="brand-mark" />
        <div>
          <div className="brand-name">Distri ARC Orbit · Stage 00</div>
          <div className="brand-sub">Orbit · AI-native</div>
        </div>
      </div>
      <div className="card" style={{ maxWidth: 420 }}>
        <div className="card-h"><h2>Status</h2></div>
        <p>
          Database: <span className={`pill ${health?.db === 'ok' ? 'good' : 'bad'}`}>{health?.db ?? '…'}</span>{' '}
          Antrean: <span className={`pill ${health?.queue === 'ok' ? 'good' : 'bad'}`}>{health?.queue ?? '…'}</span>
        </p>
      </div>
    </main>
  )
}
