import { useEffect, useState, type ReactNode } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'

/** What the shell hands its pages: the page-header slot Chat puts its numbers bar in. */
export type ShellContext = { topbarSlot: HTMLElement | null }
import sprite from '../icons/sprite.svg?raw'
import { Icon } from '../components/Icon'
import { AppearancePicker } from '../components/AppearancePicker'
import { useAppearance } from './appearance'
import { useFeedback } from '../components/feedback'
import { TITLES, type ScreenKey } from '../lib/i18n/id'
import { LoadingPopup } from '../components/LoadingPopup'
import { todayLine } from '../lib/format'
import { useHealth, useMe, useNow, useOrbit, useWAStatus } from './queries'
import { Dock } from './Dock'
import { OrchPill, useOrchStatus } from './orch'
import { useSse } from './SseProvider'

export const ROUTES: Record<ScreenKey, string> = {
  today: '/',
  dashboard: '/dashboard',
  orch: '/orchestrator',
  chat: '/chat',
  reportwa: '/report-whatsapp',
  orbit: '/orbit',
  kuad: '/orbit/segmen',
  net: '/orbit/relasi',
  dealer: '/dealer',
  stock: '/stok',
  ar: '/kredit',
  users: '/pengguna',
  roles: '/peran',
  branches: '/cabang',
  salesmap: '/mapping-sales',
  mcp: '/claude',
  aiapi: '/pengaturan/api-ai',
  aitrigger: '/pengaturan/ai-trigger',
  conn: '/pengaturan',
  konsep: '/panduan',
}

export function screenOf(path: string): ScreenKey {
  if (path === '/' || path === '') return 'today'
  if (path.startsWith('/dashboard')) return 'dashboard'
  if (path.startsWith('/orchestrator')) return 'orch'
  if (path.startsWith('/chat')) return 'chat'
  if (path.startsWith('/report-whatsapp')) return 'reportwa'
  if (path.startsWith('/orbit/segmen')) return 'kuad'
  if (path.startsWith('/orbit/relasi')) return 'net'
  if (path.startsWith('/orbit')) return 'orbit'
  if (path.startsWith('/dealer')) return 'dealer'
  if (path.startsWith('/stok')) return 'stock'
  if (path.startsWith('/kredit')) return 'ar'
  if (path.startsWith('/pengguna')) return 'users'
  if (path.startsWith('/peran')) return 'roles'
  if (path.startsWith('/cabang')) return 'branches'
  if (path.startsWith('/mapping-sales')) return 'salesmap'
  if (path.startsWith('/claude')) return 'mcp'
  if (path.startsWith('/pengaturan/api-ai')) return 'aiapi'
  if (path.startsWith('/pengaturan/ai-trigger')) return 'aitrigger'
  if (path.startsWith('/pengaturan')) return 'conn'
  if (path.startsWith('/panduan')) return 'konsep'
  return 'today'
}

/** A screen the user may open: Mapping sales goes with the Pengguna or Pengaturan menu (no role setting of its own). */
export function canSee(screens: string[], key: ScreenKey): boolean {
  if (key === 'salesmap') return screens.includes('users') || screens.includes('conn')
  if (key === 'aiapi') return screens.includes('conn') || screens.includes('mcp')
  if (key === 'aitrigger') return screens.includes('conn') || screens.includes('chat')
  return screens.includes(key === 'dashboard' ? 'today' : key)
}

function NavBtn({ to, cur, icon, children, badge }: { to: ScreenKey; cur: ScreenKey; icon: string; children: ReactNode; badge?: ReactNode }) {
  const nav = useNavigate()
  const { data: me } = useMe()
  if (me && !canSee(me.screens, to)) return null
  const hl = cur === 'net' || cur === 'kuad' ? 'orbit' : cur
  return (
    <button className={`nav-btn ${hl === to ? 'is-active' : ''}`} title={typeof children === 'string' ? children : undefined} onClick={() => nav(ROUTES[to])}>
      <Icon name={icon} />
      <span className="nav-label">{children}</span>
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
  // unread chats from the numbers' counts (/wa/status), never by loading every chat on every page (Sam 19 Oct: slow)
  const { data: wa } = useWAStatus()
  const unread = me?.screens.includes('chat') ? wa?.items.reduce((a, n) => a + (n.unread_count ?? 0), 0) : undefined
  const orch = useOrchStatus()
  const { toast } = useFeedback()

  useEffect(() => {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [cur])
  useEffect(() => {
    // the consent page explains itself to people whose role cannot connect Claude
    if (me && !canSee(me.screens, cur) && loc.pathname !== '/claude/izin') nav('/', { replace: true })
  }, [me, cur, nav, loc.pathname])
  const [menu, setMenu] = useState(false)
  const [topbarSlot, setTopbarSlot] = useState<HTMLElement | null>(null) // Chat's numbers bar renders here
  const { collapsed, setCollapsed } = useAppearance()
  const { subscribe } = useSse()
  useEffect(
    () =>
      subscribe((name, data) => {
        if (name !== 'outbox_failed') return
        const d = (data ?? {}) as { channel?: string; error?: string }
        toast(`${d.channel === 'wa' ? 'Pesan WhatsApp' : 'Penulisan ke Odoo'} gagal: ${d.error ?? 'coba lagi'} — saran tetap disetujui, worker mencoba ulang`)
      }),
    [subscribe, toast],
  )
  const logout = async () => {
    await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' })
    window.location.assign('/login')
  }

  const atRisk = dealers?.filter((d) => d.metrics.status === 'At risk' || d.metrics.status === 'Churn').length
  const creditBad = dealers?.filter((d) => d.credit_limit > 0 && (d.metrics.credit.state === 'over limit' || d.metrics.credit.state === 'overdue')).length
  const [title, sub] = TITLES[cur]
  const subline = cur === 'today' ? todayLine(now) : sub
  const isOrbit = cur === 'orbit' || cur === 'net' || cur === 'kuad'
  // the menu scrolls on short screens: keep the open page's item in view
  useEffect(() => { document.querySelector('.rail .nav-btn.is-active')?.scrollIntoView?.({ block: 'nearest' }) }, [cur, me])

  return (
    <>
      <div style={{ position: 'absolute', width: 0, height: 0, overflow: 'hidden' }} dangerouslySetInnerHTML={{ __html: sprite }} />
      <div className="app">
        <aside className="rail">
          <div className="brand">
            <div className="brand-mark" />
            <div><div className="brand-name">GSI Orbit</div><div className="brand-sub">CRM Distribusi · AI</div></div>
          </div>
          <nav className="nav" aria-label="Navigasi utama">
            <div className="nav-sec">Kendali</div>
            <NavBtn to="dashboard" cur={cur} icon="chart">Dashboard</NavBtn>
            <NavBtn to="today" cur={cur} icon="sun" badge={<Badge n={orch.pending} />}>Pusat kendali</NavBtn>
            <NavBtn to="orch" cur={cur} icon="spark" badge={<span className="live" title="siklus berjalan tiap jam" />}>Orchestrator</NavBtn>
            <NavBtn to="chat" cur={cur} icon="chat" badge={<Badge n={unread} color="var(--good)" />}>Chat</NavBtn>
            <NavBtn to="reportwa" cur={cur} icon="trend">Report WhatsApp</NavBtn>
            <div className="nav-sec">Dealer</div>
            <NavBtn to="orbit" cur={cur} icon="target" badge={<Badge n={atRisk} color="var(--bad)" />}>Orbit</NavBtn>
            <NavBtn to="dealer" cur={cur} icon="building">Dealer</NavBtn>
            <div className="nav-sec">Operasi</div>
            <NavBtn to="stock" cur={cur} icon="box">Push stok</NavBtn>
            <NavBtn to="ar" cur={cur} icon="cash" badge={<Badge n={creditBad} color="var(--bad)" />}>Kredit · kas</NavBtn>
            {me && ['users', 'roles', 'branches', 'conn'].some((k) => me.screens.includes(k)) && <div className="nav-sec">Master data</div>}
            <NavBtn to="users" cur={cur} icon="people">Pengguna</NavBtn>
            <NavBtn to="roles" cur={cur} icon="shield">Peran &amp; akses</NavBtn>
            <NavBtn to="branches" cur={cur} icon="building">Cabang</NavBtn>
            <NavBtn to="salesmap" cur={cur} icon="net">Mapping sales</NavBtn>
            {me && (['mcp', 'aiapi', 'aitrigger', 'konsep', 'conn'] as ScreenKey[]).some((k) => canSee(me.screens, k)) && <div className="nav-sec">Pengaturan</div>}
            <NavBtn to="mcp" cur={cur} icon="spark">MCP Claude</NavBtn>
            <NavBtn to="aiapi" cur={cur} icon="plug">API &amp; model AI</NavBtn>
            <NavBtn to="aitrigger" cur={cur} icon="spark">AI Trigger</NavBtn>
            <NavBtn to="konsep" cur={cur} icon="doc">Panduan</NavBtn>
            <NavBtn to="conn" cur={cur} icon="gear">Pengaturan</NavBtn>
          </nav>
          <div className="rail-sec">
            <div style={{ position: 'relative' }}>
              {menu && (
                <div className="card me-menu">
                  <AppearancePicker compact />
                </div>
              )}
              <div className="me">
                <div className="me-open" role="button" tabIndex={0} aria-expanded={menu} aria-label={`Akun ${me?.name ?? ''} · tema & warna sidebar`} title={me ? `${me.name} · ${roleLine(me.role, me.branch)}` : undefined} onClick={() => setMenu(!menu)} onKeyDown={(e) => e.key === 'Enter' && setMenu(!menu)}>
                  <div className="avatar">{me ? initialsOf(me.name) : '··'}</div>
                  <div className="me-who"><b>{me?.name ?? '…'}</b><span>{me?.email ?? ''}</span></div>
                </div>
                <button className="me-out" aria-label="Keluar" title="Keluar" onClick={() => void logout()}><Icon name="logout" /></button>
              </div>
            </div>
          </div>
          <button className="rail-toggle" aria-label={collapsed ? 'Lebarkan sidebar' : 'Ringkas sidebar'} title={collapsed ? 'Lebarkan sidebar' : 'Ringkas sidebar'} onClick={() => { setCollapsed(!collapsed); setMenu(false) }}><Icon name="chev" /></button>
        </aside>

        <main className="stage">
          <header className="topbar">
            <div className="tb-left">
              <h1>{title}</h1>
              <div className="sub">{subline}</div>
              {isOrbit && me?.screens.includes('net') !== false && (
                <div className="vtabs">
                  <button className={cur === 'orbit' ? 'is-active' : ''} onClick={() => nav(ROUTES.orbit)}><Icon name="target" />Orbit</button>
                  <button className={cur === 'kuad' ? 'is-active' : ''} onClick={() => nav(ROUTES.kuad)}><Icon name="chart" />Segmen</button>
                  <button className={cur === 'net' ? 'is-active' : ''} onClick={() => nav(ROUTES.net)}><Icon name="net" />Peta relasi 3D</button>
                </div>
              )}
            </div>
            {cur === 'chat' && <div className="tb-chat" ref={setTopbarSlot} />}
            <div className="top-right">
              {cur !== 'chat' && <OrchPill />}
              {me?.pilot_mode === 'shadow' && <span className="chip-sample" title="Pilot: Orchestrator menganalisis, tidak ada yang dikirim ke dealer">Mode bayangan</span>}
              {health?.sample_data && <span className="chip-sample">Data contoh</span>}
              <button className="icon-btn" aria-label="Notifikasi" onClick={() => toast('Semua yang butuh Anda ada di Pusat kendali.')}>
                <Icon name="bell" />
                <span className="nd" />
              </button>
            </div>
          </header>
          <section className="screen" key={cur}>
            <Outlet context={{ topbarSlot } satisfies ShellContext} />
          </section>
        </main>

        <nav className="tabbar" aria-label="Navigasi">
          {([['dashboard', 'chart', 'Dashboard'], ['today', 'sun', 'Kendali'], ['orch', 'spark', 'Orchestrator'], ['orbit', 'target', 'Orbit'], ['dealer', 'building', 'Dealer'], ['chat', 'chat', 'Chat']] as const).filter(([k]) => !me || me.screens.includes(k === 'dashboard' ? 'today' : k)).map(([k, ic, label]) => (
            <button key={k} className={(isOrbit ? 'orbit' : cur) === k ? 'is-active' : ''} onClick={() => nav(ROUTES[k])}>
              <Icon name={ic} />
              {label}
            </button>
          ))}
        </nav>
      </div>
      {cur !== 'today' && cur !== 'orch' && cur !== 'dashboard' && cur !== 'chat' && <Dock screen={cur} />}
      <LoadingPopup />
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
