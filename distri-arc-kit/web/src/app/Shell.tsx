import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import sprite from '../icons/sprite.svg?raw'
import { Icon } from '../components/Icon'
import { useFeedback } from '../components/feedback'
import { AGENT_NAMES, TITLES, type ScreenKey } from '../lib/i18n/id'
import { todayLine } from '../lib/format'
import { useHealth, useMe, useNow, useOrbit, useThreads } from './queries'
import { useCommand } from './command'
import { Dock } from './Dock'
import { OrchPill, useOrchStatus } from './orch'

export const ROUTES: Record<ScreenKey, string> = {
  today: '/',
  orch: '/orchestrator',
  chat: '/chat',
  orbit: '/orbit',
  kuad: '/orbit/segmen',
  net: '/orbit/relasi',
  dealer: '/dealer',
  stock: '/stok',
  ar: '/kredit',
  conn: '/pengaturan',
  konsep: '/panduan',
}

export function screenOf(path: string): ScreenKey {
  if (path === '/' || path === '') return 'today'
  if (path.startsWith('/orchestrator')) return 'orch'
  if (path.startsWith('/chat')) return 'chat'
  if (path.startsWith('/orbit/segmen')) return 'kuad'
  if (path.startsWith('/orbit/relasi')) return 'net'
  if (path.startsWith('/orbit')) return 'orbit'
  if (path.startsWith('/dealer')) return 'dealer'
  if (path.startsWith('/stok')) return 'stock'
  if (path.startsWith('/kredit')) return 'ar'
  if (path.startsWith('/pengaturan')) return 'conn'
  if (path.startsWith('/panduan')) return 'konsep'
  return 'today'
}

function NavBtn({ to, cur, icon, children, badge }: { to: ScreenKey; cur: ScreenKey; icon: string; children: ReactNode; badge?: ReactNode }) {
  const nav = useNavigate()
  const hl = cur === 'net' || cur === 'kuad' ? 'orbit' : cur
  return (
    <button className={`nav-btn ${hl === to ? 'is-active' : ''}`} onClick={() => nav(ROUTES[to])}>
      <Icon name={icon} />
      {children}
      {badge}
    </button>
  )
}

function Badge({ n, color }: { n?: number; color?: string }) {
  if (!n) return null
  return <span className="badge" style={color ? { background: color } : undefined}>{n}</span>
}

export function Shell() {
  const loc = useLocation()
  const nav = useNavigate()
  const cur = screenOf(loc.pathname)
  const { data: me } = useMe()
  const { data: health } = useHealth()
  const now = useNow()
  const { data: dealers } = useOrbit()
  const { data: threads } = useThreads('all')
  const unread = threads?.reduce((a, t) => a + t.unread, 0)
  const orch = useOrchStatus()
  const run = useCommand()
  const inputRef = useRef<HTMLInputElement>(null)
  const [q, setQ] = useState('')
  const { toast } = useFeedback()

  useEffect(() => {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [cur])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        inputRef.current?.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  const atRisk = dealers?.filter((d) => d.metrics.status === 'At risk' || d.metrics.status === 'Churn').length
  const creditBad = dealers?.filter((d) => d.credit_limit > 0 && (d.metrics.credit.state === 'over limit' || d.metrics.credit.state === 'overdue')).length
  const [title, sub] = TITLES[cur]
  const subline = cur === 'today' ? todayLine(now) : sub
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const v = q.trim()
    if (!v) return
    setQ('')
    run(v)
  }
  const isOrbit = cur === 'orbit' || cur === 'net' || cur === 'kuad'

  return (
    <>
      <div style={{ position: 'absolute', width: 0, height: 0, overflow: 'hidden' }} dangerouslySetInnerHTML={{ __html: sprite }} />
      <div className="app">
        <aside className="rail">
          <div className="brand">
            <div className="brand-mark" />
            <div><div className="brand-name">Distri ARC</div><div className="brand-sub">Orbit · AI-native</div></div>
          </div>
          <nav className="nav" aria-label="Navigasi utama">
            <div className="nav-sec">Kendali</div>
            <NavBtn to="today" cur={cur} icon="sun" badge={<Badge n={orch.pending} />}>Pusat kendali</NavBtn>
            <NavBtn to="orch" cur={cur} icon="spark" badge={<span className="live" title="siklus berjalan tiap jam" />}>Orchestrator</NavBtn>
            <NavBtn to="chat" cur={cur} icon="chat" badge={<Badge n={unread} color="var(--good)" />}>Chat</NavBtn>
            <div className="nav-sec">Dealer</div>
            <NavBtn to="orbit" cur={cur} icon="target" badge={<Badge n={atRisk} color="var(--bad)" />}>Orbit</NavBtn>
            <NavBtn to="dealer" cur={cur} icon="building">Dealer</NavBtn>
            <div className="nav-sec">Operasi</div>
            <NavBtn to="stock" cur={cur} icon="box">Push stok</NavBtn>
            <NavBtn to="ar" cur={cur} icon="cash" badge={<Badge n={creditBad} color="var(--bad)" />}>Kredit · kas</NavBtn>
          </nav>
          <div className="rail-sec">
            <nav className="nav" aria-label="Pengaturan">
              <NavBtn to="konsep" cur={cur} icon="doc">Panduan</NavBtn>
              <NavBtn to="conn" cur={cur} icon="gear">Pengaturan</NavBtn>
            </nav>
            <div className="agents">
              <div className="agents-h"><span className="dot-live" />Orchestrator · <span>{orch.running ? 'menganalisis…' : 'siap'}</span></div>
              <ul>{AGENT_NAMES.map((a) => <li key={a} className="on">{a}</li>)}</ul>
            </div>
            <div className="me">
              <div className="avatar">{me ? initialsOf(me.name) : '··'}</div>
              <div><b>{me?.name ?? '…'}</b><span>{me ? roleLine(me.role, me.branch) : ''}</span></div>
            </div>
          </div>
        </aside>

        <main className="stage">
          <header className="topbar">
            <div className="tb-left">
              <h1>{title}</h1>
              <div className="sub">{subline}</div>
              {isOrbit && (
                <div className="vtabs">
                  <button className={cur === 'orbit' ? 'is-active' : ''} onClick={() => nav(ROUTES.orbit)}><Icon name="target" />Orbit</button>
                  <button className={cur === 'kuad' ? 'is-active' : ''} onClick={() => nav(ROUTES.kuad)}><Icon name="chart" />Segmen</button>
                  <button className={cur === 'net' ? 'is-active' : ''} onClick={() => nav(ROUTES.net)}><Icon name="net" />Peta relasi 3D</button>
                </div>
              )}
            </div>
            <div className="top-right">
              <div className="searchwrap">
                <form className="search" autoComplete="off" onSubmit={submit}>
                  <Icon name="spark" />
                  <input ref={inputRef} value={q} onChange={(e) => setQ(e.target.value)} placeholder="Tanya atau perintahkan Orchestrator… mis. “analisis ulang Mitra Jaya”" aria-label="Tanya" />
                  <kbd>⌘K</kbd>
                </form>
              </div>
              <OrchPill />
              {health?.sample_data && <span className="chip-sample">Data contoh</span>}
              <button className="icon-btn" aria-label="Notifikasi" onClick={() => toast('Semua yang butuh Anda ada di Pusat kendali.')}>
                <Icon name="bell" />
                <span className="nd" />
              </button>
            </div>
          </header>
          <section className="screen" key={cur}>
            <Outlet />
          </section>
        </main>

        <nav className="tabbar" aria-label="Navigasi">
          {([['today', 'sun', 'Kendali'], ['orch', 'spark', 'Orchestrator'], ['orbit', 'target', 'Orbit'], ['dealer', 'building', 'Dealer'], ['chat', 'chat', 'Chat']] as const).map(([k, ic, label]) => (
            <button key={k} className={(isOrbit ? 'orbit' : cur) === k ? 'is-active' : ''} onClick={() => nav(ROUTES[k])}>
              <Icon name={ic} />
              {label}
            </button>
          ))}
        </nav>
      </div>
      {cur !== 'chat' && cur !== 'today' && cur !== 'orch' && <Dock screen={cur} />}
    </>
  )
}

function initialsOf(name: string) {
  const f = name.split(/\s+/).filter(Boolean)
  return f.length > 1 ? (f[0][0] + f[1][0]).toUpperCase() : name.slice(0, 2).toUpperCase()
}

function roleLine(role: string, branch: string) {
  const r: Record<string, string> = { ceo: 'CEO', sales: 'Sales', admin: 'Admin', finance: 'Finance', warehouse: 'Gudang' }
  return `${r[role] ?? role} · ${branch.toLowerCase() === 'semua cabang' ? 'semua cabang' : 'cabang ' + branch}`
}
