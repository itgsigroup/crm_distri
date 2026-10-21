import type { ComponentType } from 'react'
import { createBrowserRouter } from 'react-router'
import { Shell } from './Shell'
import { FeedbackProvider } from '../components/feedback'
import { OrchProvider } from './orch'
import { ControlCenter } from '../features/control/ControlCenter'

// Every page but Pusat kendali (the home screen) is its own chunk, loaded when it is opened and prefetched while the
// browser is idle (prefetchPages): the first load no longer carries three.js, the chat, the relation galaxy and the
// guide for every user (one 1.6 MB bundle, 20 Oct).
type Loader = () => Promise<ComponentType>
const PAGES: Record<string, Loader> = {
  login: () => import('../features/login/LoginPage').then((m) => m.LoginPage),
  dashboard: () => import('../features/dashboard/DashboardPage').then((m) => m.DashboardPage),
  orchestrator: () => import('../features/orchestrator/OrchestratorPage').then((m) => m.OrchestratorPage),
  chat: () => import('../features/chat/ChatPage').then((m) => m.ChatPage),
  reportwa: () => import('../features/reportwa/ReportWAPage').then((m) => m.ReportWAPage),
  orbit: () => import('../features/orbit/OrbitPage').then((m) => m.OrbitPage),
  segmen: () => import('../features/segmen/SegmenPage').then((m) => m.SegmenPage),
  relasi: () => import('../features/relasi/RelasiPage').then((m) => m.RelasiPage),
  dealer: () => import('../features/dealer/DealerPage').then((m) => m.DealerPage),
  stock: () => import('../features/stock/StockPage').then((m) => m.StockPage),
  stockcheck: () => import('../features/stock/StockCheckPage').then((m) => m.StockCheckPage),
  credit: () => import('../features/credit/CreditPage').then((m) => m.CreditPage),
  users: () => import('../features/master/UsersPage').then((m) => m.UsersPage),
  roles: () => import('../features/master/RolesPage').then((m) => m.RolesPage),
  branches: () => import('../features/master/BranchesPage').then((m) => m.BranchesPage),
  salesmap: () => import('../features/salesmap/SalesMapPage').then((m) => m.SalesMapPage),
  claude: () => import('../features/claude/ClaudePage').then((m) => m.ClaudePage),
  consent: () => import('../features/claude/ConsentPage').then((m) => m.ConsentPage),
  settings: () => import('../features/settings/SettingsPage').then((m) => m.SettingsPage),
  pilot: () => import('../features/pilot/PilotPage').then((m) => m.PilotPage),
  data: () => import('../features/data/DataPage').then((m) => m.DataPage),
  aiapi: () => import('../features/aiapi/AIAPIPage').then((m) => m.AIAPIPage),
  aitrigger: () => import('../features/aitrigger/AITriggerPage').then((m) => m.AITriggerPage),
  guide: () => import('../features/guide/GuidePage').then((m) => m.GuidePage),
}

// After a deploy an open tab still asks for the old chunk names, which are gone: reload once to get the new ones.
const RELOADED = 'chunk-reload'
async function load(key: string) {
  try {
    const Component = await PAGES[key]()
    try { sessionStorage.removeItem(RELOADED) } catch { /* private mode */ }
    return { Component }
  } catch (e) {
    let again = false
    try { again = !sessionStorage.getItem(RELOADED); sessionStorage.setItem(RELOADED, '1') } catch { /* private mode */ }
    if (again) window.location.reload()
    throw e
  }
}
const page = (key: string) => ({ lazy: () => load(key) })

/** Fetch the other pages' chunks while the browser is idle, so opening them is instant. */
export function prefetchPages() {
  const keys = Object.keys(PAGES).filter((k) => k !== 'login')
  const next = () => {
    const k = keys.shift()
    if (!k) return
    PAGES[k]().catch(() => {}).finally(() => idle(next))
  }
  idle(next)
}
const idle = (f: () => void) => ('requestIdleCallback' in window ? window.requestIdleCallback(f, { timeout: 3000 }) : setTimeout(f, 300))

function Booting() {
  return <div className="boot-fallback" role="status" aria-label="Memuat halaman" />
}

// Routes of docs/design/08-frontend.md.
export const router = createBrowserRouter([
  { path: '/login', ...page('login'), hydrateFallbackElement: <Booting /> },
  {
    path: '/',
    // inside the router so sheets can navigate (provenance chips, "Buka dealer")
    element: <FeedbackProvider><OrchProvider><Shell /></OrchProvider></FeedbackProvider>,
    hydrateFallbackElement: <Booting />,
    children: [
      { index: true, element: <ControlCenter /> },
      { path: 'dashboard', ...page('dashboard') },
      { path: 'orchestrator', ...page('orchestrator') },
      { path: 'chat/:threadId?', ...page('chat') },
      { path: 'report-whatsapp', ...page('reportwa') },
      { path: 'orbit', ...page('orbit') },
      { path: 'orbit/segmen', ...page('segmen') },
      { path: 'orbit/relasi', ...page('relasi') },
      { path: 'dealer/:id?', ...page('dealer') },
      { path: 'stok', ...page('stock') },
      { path: 'stok/cek', ...page('stockcheck') },
      { path: 'kredit', ...page('credit') },
      { path: 'pengguna', ...page('users') },
      { path: 'peran', ...page('roles') },
      { path: 'cabang', ...page('branches') },
      { path: 'mapping-sales', ...page('salesmap') },
      { path: 'claude', ...page('claude') },
      { path: 'claude/izin', ...page('consent') },
      { path: 'pengaturan', ...page('settings') },
      { path: 'pengaturan/pilot', ...page('pilot') },
      { path: 'pengaturan/data', ...page('data') },
      { path: 'pengaturan/api-ai', ...page('aiapi') },
      { path: 'pengaturan/ai-trigger', ...page('aitrigger') },
      { path: 'panduan', ...page('guide') },
    ],
  },
])
