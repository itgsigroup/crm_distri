import { useNavigate } from 'react-router'
import { Icon } from '../../components/Icon'

/** Screens whose stage has not been built yet show what arrives and when. */
export function Upcoming({ title, stage, text }: { title: string; stage: string; text: string }) {
  const nav = useNavigate()
  return (
    <div className="card" style={{ maxWidth: 640 }}>
      <div className="card-h"><h2>{title}</h2><span className="pill neutral" style={{ marginLeft: 'auto' }}>Stage {stage}</span></div>
      <p style={{ fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.55 }}>{text}</p>
      <div style={{ display: 'flex', gap: 8, marginTop: 14 }}>
        <button className="btn ghost" onClick={() => nav('/')}><Icon name="sun" />Pusat kendali</button>
        <button className="btn ghost" onClick={() => nav('/orbit')}><Icon name="target" />Orbit</button>
      </div>
    </div>
  )
}
