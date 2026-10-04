import { useEffect, useRef, useState, type FormEvent } from 'react'
import { api, useApi } from './api/client'
import type { AccountListItem, Me, ScreenKey, Shell } from './api/types'
import { ActionSheet, Toast } from './components/ActionSheet'
import { Icon } from './components/ui'
import { Login } from './screens/Login'
import { TodayScreen } from './screens/Today'
import { AskScreen } from './screens/Ask'
import { ChatScreen } from './screens/Chat'
import { RelasiScreen } from './screens/Relasi'
import { NetworkScreen } from './screens/Network'
import { PipelineScreen } from './screens/Pipeline'
import { ProspekScreen } from './screens/Prospek'
import { KasScreen } from './screens/Kas'
import { SettingsScreen, CONN_TABS } from './screens/Settings'
import { useUI } from './state/ui'

const NAV_PARENT: Partial<Record<ScreenKey, ScreenKey>> = { pros: 'pipe' }

export default function App() {
  const { me, setMe } = useUI()
  const [checked, setChecked] = useState(false)
  useEffect(() => {
    api.get<Me>('/api/me').then(setMe).catch(() => setMe(null)).finally(() => setChecked(true))
    const onUnauth = () => setMe(null)
    window.addEventListener('arc:unauthorized', onUnauth)
    return () => window.removeEventListener('arc:unauthorized', onUnauth)
  }, [setMe])
  if (!checked) return null
  if (!me) return <Login />
  return <Shell_ />
}

function Shell_() {
  const { route, go, me, toast, refreshKey } = useUI()
  const { data: shell, reload } = useApi<Shell>('/api/shell')
  useEffect(() => { reload() }, [refreshKey, reload])

  // Delegated handler for server-rendered HTML (brief points, verdicts) with data-go / data-toast.
  useEffect(() => {
    const on = (e: MouseEvent) => {
      const t = e.target as HTMLElement
      const g = t.closest('[data-go]') as HTMLElement | null
      if (g && g.closest('.html-go')) { e.preventDefault(); go(g.dataset.go!); return }
      const ts = t.closest('[data-toast]') as HTMLElement | null
      if (ts && ts.closest('.html-go')) toast(ts.dataset.toast!)
    }
    document.addEventListener('click', on)
    return () => document.removeEventListener('click', on)
  }, [go, toast])

  const screen = route.screen
  const hl = NAV_PARENT[screen] || screen
  const title = shell?.screens[screen]
  const b = shell?.badges

  return (
    <div className="app">
      <aside className="rail">
        <div className="brand">
          <div className="brand-mark" />
          <div><div className="brand-name">ARC</div><div className="brand-sub">Relationship Core</div></div>
        </div>
        <nav className="nav" aria-label="Navigasi utama">
          <div className="nav-sec">Kerja hari ini</div>
          <NavBtn k="today" hl={hl} icon="i-sun" label="Hari ini" badge={b?.today} />
          <NavBtn k="chat" hl={hl} icon="i-chat" label="Chat" badge={b?.chat} color="var(--good)" />
          <div className="nav-sec">Bisnis</div>
          <NavBtn k="rel" hl={hl} icon="i-people" label="Relasi" />
          <NavBtn k="net" hl={hl} icon="i-net" label="Peta 3D" />
          <NavBtn k="pipe" hl={hl} icon="i-chart" label="Penjualan" />
          <NavBtn k="cash" hl={hl} icon="i-cash" label="Kas" badge={b?.cash} color="var(--bad)" />
        </nav>
        <div className="rail-sec">
          <nav className="nav" aria-label="Pengaturan">
            <NavBtn k="conn" hl={hl} icon="i-gear" label="Pengaturan" badge={b?.conn} color="var(--warn)" />
          </nav>
          <div className="agents">
            <div className="agents-h"><span className="dot-live" />{shell?.agents.active ?? 0} agen aktif</div>
            <ul>{shell?.agents.list.map(a => <li key={a.name} className={a.on ? 'on' : ''}>{a.name}</li>)}</ul>
          </div>
          <div className="me">
            <div className="avatar">{me?.initials}</div>
            <div><b>{me?.name}</b><span>{me?.role_label}</span></div>
          </div>
        </div>
      </aside>

      <main className="stage">
        <header className="topbar">
          <div className="tb-left">
            <h1 id="screen-title">{title?.title ?? ''}</h1>
            <div className="sub" id="screen-sub">{title?.sub ?? ''}</div>
            <ViewTabs screen={screen} param={route.param} />
          </div>
          <div className="top-right">
            <TopAsk screen={screen} screenTitle={title?.title ?? ''} />
            {shell?.sample_data && <span className="chip-sample">Data contoh</span>}
            <button className="icon-btn" aria-label="Notifikasi" onClick={() => toast('Semua yang butuh Anda ada di Hari ini — tidak ada inbox terpisah.')}>
              <Icon n="i-bell" /><span className="nd" />
            </button>
          </div>
        </header>
        <Screen screen={screen} />
      </main>

      <nav className="tabbar" aria-label="Navigasi">
        {([['today', 'i-sun', 'Hari ini'], ['chat', 'i-chat', 'Chat'], ['rel', 'i-people', 'Relasi'], ['pipe', 'i-chart', 'Penjualan'], ['cash', 'i-cash', 'Kas']] as const).map(([k, ic, l]) => (
          <button key={k} className={hl === k ? 'is-active' : ''} onClick={() => go(k)}><Icon n={ic} />{l}</button>
        ))}
      </nav>
      <ActionSheet />
      <Toast />
    </div>
  )
}

function NavBtn({ k, hl, icon, label, badge, color }: { k: ScreenKey; hl: ScreenKey; icon: string; label: string; badge?: number; color?: string }) {
  const { go } = useUI()
  return (
    <button className={'nav-btn' + (hl === k ? ' is-active' : '')} onClick={() => go(k)}>
      <Icon n={icon} />{label}
      {!!badge && <span className="badge" style={color ? { background: color } : undefined}>{badge}</span>}
    </button>
  )
}

function Screen({ screen }: { screen: ScreenKey }) {
  switch (screen) {
    case 'today': return <TodayScreen />
    case 'ask': return <AskScreen />
    case 'chat': return <ChatScreen />
    case 'rel': return <RelasiScreen />
    case 'net': return <NetworkScreen />
    case 'pipe': return <PipelineScreen />
    case 'pros': return <ProspekScreen />
    case 'cash': return <KasScreen />
    case 'conn': return <SettingsScreen />
  }
}

function ViewTabs({ screen, param }: { screen: ScreenKey; param: string }) {
  const { go } = useUI()
  if (screen === 'pipe' || screen === 'pros') {
    return (
      <div className="vtabs" id="vtabs">
        <button className={screen === 'pipe' ? 'is-active' : ''} onClick={() => go('pipe')}><Icon n="i-kanban" />Pipeline</button>
        <button className={screen === 'pros' ? 'is-active' : ''} onClick={() => go('pros')}><Icon n="i-radar" />Prospek &amp; funnel</button>
      </div>
    )
  }
  if (screen === 'rel' || screen === 'net') {
    return (
      <div className="vtabs" id="vtabs">
        <button className={screen === 'rel' ? 'is-active' : ''} onClick={() => go('rel')}><Icon n="i-people" />Daftar akun</button>
        <button className={screen === 'net' ? 'is-active' : ''} onClick={() => go('net')}><Icon n="i-net" />Peta koneksi 3D</button>
      </div>
    )
  }
  if (screen === 'conn') {
    const active = param || 'sec-sumber'
    return (
      <div className="vtabs" id="vtabs">
        {CONN_TABS.map(([id, l]) => (
          <button key={id} className={active === id ? 'is-active' : ''} onClick={() => go('conn:' + id)}>{l}</button>
        ))}
      </div>
    )
  }
  return null
}

// "Tanya ARC atau cari akun…" box with contextual questions (⌘K).
function TopAsk({ screen, screenTitle }: { screen: ScreenKey; screenTitle: string }) {
  const { ask, go, toast } = useUI()
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [chips, setChips] = useState<string[]>([])
  const input = useRef<HTMLInputElement>(null)
  const wrap = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); input.current?.focus() }
    }
    const onClick = (e: MouseEvent) => { if (wrap.current && !wrap.current.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('keydown', onKey)
    document.addEventListener('click', onClick)
    return () => { document.removeEventListener('keydown', onKey); document.removeEventListener('click', onClick) }
  }, [])
  const [openedOn, setOpenedOn] = useState(screen)
  if (openedOn !== screen) {
    setOpenedOn(screen)
    setOpen(false)
  }

  const onFocus = () => {
    api.get<string[]>('/api/ask/suggestions?screen=' + screen).then(setChips).catch(() => setChips([]))
    setOpen(true)
  }
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const text = q.trim()
    if (!text) return
    setQ('')
    setOpen(false)
    if (text.length > 3 && !text.includes('?')) {
      try {
        const r = await api.get<{ items: AccountListItem[] }>('/api/accounts?q=' + encodeURIComponent(text))
        const acc = r.items.find(a => a.name.toLowerCase().includes(text.toLowerCase()))
        if (acc) { go('rel:' + acc.id); toast('Membuka ' + acc.name); return }
      } catch { /* fall through to Ask */ }
    }
    ask(text)
  }
  return (
    <div className="searchwrap" ref={wrap}>
      <form className="search" id="topask" autoComplete="off" onSubmit={submit}>
        <Icon n="i-spark" />
        <input ref={input} id="topask-input" placeholder="Tanya ARC atau cari akun…" aria-label="Tanya ARC" value={q} onChange={e => setQ(e.target.value)} onFocus={onFocus} />
        <kbd>⌘K</kbd>
      </form>
      <div className={'search-dd' + (open ? ' show' : '')} id="search-dd">
        <div className="dd-h">Tanya tentang {screenTitle}</div>
        <div className="chips">
          {chips.map(c => <button type="button" key={c} className="chip" onClick={() => { setOpen(false); ask(c) }}>{c}</button>)}
        </div>
        <div className="dd-f">
          <button className="btn quiet" style={{ height: 26, fontSize: 12 }} onClick={() => go('ask')}>Buka percakapan lengkap <Icon n="i-arrow" /></button>
        </div>
      </div>
    </div>
  )
}
