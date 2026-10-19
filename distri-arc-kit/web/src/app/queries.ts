import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type Items } from '../api/client'
import type { AgentInfo, AutonomyPolicy, Conflict, Cycle, CycleLatest, AnalystInfo, CronPreview, MCPCall, MCPClient, MCPInfo, MCPPolicy, Schedule, ScheduleRun, ScheduleRunFull, Plan, Proposal, Relasi, RelasiInsight } from '../api/types'
import type { DealerAnalysis, ScreenAnalysis, Trend, WAReport } from '../api/types'
import type { ChatContext, InternalNumber, ThreadDetail, ThreadView, WAAnalysis, WAGroup, WANumber } from '../api/types'
import type {
  AgendaRow, AgingItem, ARRow, BoardItem, CreditOverview, CriticalItem, ExposureRow, ForecastRow, ProductSales, Brief, DealerDetail, Health, HealthFull, KPI, Me, Mover, Sales, SegmentSummary, StatusSummary,
} from '../api/types'

// Query keys follow docs/design/08-frontend.md (Data & realtime); SSE events invalidate them.

const q = (sales?: string) => (sales && sales !== 'all' ? `?sales=${encodeURIComponent(sales)}` : '')

export const useMe = () => useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me'), staleTime: 60_000 })
export const useHealth = () => useQuery({ queryKey: ['health'], queryFn: () => api.get<Health>('/health'), staleTime: 30_000 })
/** Full health (CEO/admin): Pengaturan → Status sistem. */
export const useSystemStatus = () => useQuery({ queryKey: ['health', 'full'], queryFn: () => api.get<HealthFull>('/health'), refetchInterval: 30_000 })
export const useSales = () => useQuery({ queryKey: ['sales'], queryFn: () => api.get<Items<Sales>>('/sales').then((r) => r.items) })
export const useDealers = (term = '', type = '') =>
  useQuery({
    queryKey: ['dealers', term, type],
    queryFn: () => {
      const q = new URLSearchParams()
      if (term) q.set('q', term)
      if (type) q.set('type', type)
      const qs = q.toString()
      return api.get<Items<BoardItem>>('/dealers' + (qs ? '?' + qs : '')).then((r) => r.items)
    },
  })
export const useDealer = (id: string | undefined) =>
  useQuery({ queryKey: ['dealer', id], enabled: !!id, queryFn: () => api.get<DealerDetail>(`/dealers/${id}`) })
export const useOrbit = (sales?: string) => useQuery({ queryKey: ['orbit', sales ?? 'all'], queryFn: () => api.get<Items<BoardItem>>('/orbit' + q(sales)).then((r) => r.items) })
export const useOrbitSummary = (sales?: string) =>
  useQuery({ queryKey: ['orbit', 'summary', sales ?? 'all'], queryFn: () => api.get<{ items: StatusSummary[]; prospects: number }>('/orbit/summary' + q(sales)) })
export const useOrbitMovers = (sales?: string) =>
  useQuery({ queryKey: ['orbit', 'movers', sales ?? 'all'], queryFn: () => api.get<Items<Mover>>('/orbit/movers' + q(sales)).then((r) => r.items) })
export const useSegmen = (sales?: string) =>
  useQuery({ queryKey: ['segmen', sales ?? 'all'], queryFn: () => api.get<{ items: BoardItem[]; thresholds: { freq_per_month: number; size_idr: number } }>('/segmen' + q(sales)) })
export const useSegmenSummary = (sales?: string) =>
  useQuery({ queryKey: ['segmen', 'summary', sales ?? 'all'], queryFn: () => api.get<{ items: SegmentSummary[]; total_omzet_bln: number; prospects: number }>('/segmen/summary' + q(sales)) })
/** "?sales=andi" (or "&sales=andi"): one sales' page; nothing for everyone. */
const sp = (sales?: string, sep = '?') => (sales ? `${sep}sales=${encodeURIComponent(sales)}` : '')
export const useSegmenMovers = (sales?: string) =>
  useQuery({ queryKey: ['segmen', 'movers', sales ?? 'all'], queryFn: () => api.get<Items<Mover>>('/segmen/movers' + q(sales)).then((r) => r.items) })
export const useDue = (days = 7, sales?: string) => useQuery({ queryKey: ['dealers', 'due', days, sales ?? 'all'], queryFn: () => api.get<Items<BoardItem>>(`/dealers/due?days=${days}${sp(sales, '&')}`).then((r) => r.items) })
export const useDrift = (sales?: string) => useQuery({ queryKey: ['dealers', 'drift', sales ?? 'all'], queryFn: () => api.get<Items<BoardItem>>('/dealers/drift' + sp(sales)).then((r) => r.items) })
export const useCreditTight = (sales?: string) => useQuery({ queryKey: ['dealers', 'credit-tight', sales ?? 'all'], queryFn: () => api.get<Items<BoardItem>>('/dealers/credit-tight' + sp(sales)).then((r) => r.items) })
/** Dashboard history: 12 months, or one month day by day (month = 2026-10). */
export const useTrend = (mode: 'months' | 'days', month?: string) =>
  useQuery({ queryKey: ['trend', mode, month ?? ''], queryFn: () => api.get<Trend>(`/dashboard/trend?mode=${mode}${month ? `&month=${month}` : ''}`) })
export const useKpi = (sales?: string) => useQuery({ queryKey: ['kpi', sales ?? 'all'], queryFn: () => api.get<KPI>('/kpi' + sp(sales)) })
export const useAgenda = (sales?: string) => useQuery({ queryKey: ['agenda', sales ?? 'all'], queryFn: () => api.get<Items<AgendaRow>>('/agenda' + sp(sales)).then((r) => r.items) })
export const useBrief = (sales?: string) => useQuery({ queryKey: ['brief', 'today', sales ?? 'all'], queryFn: () => api.get<Brief>('/brief/today' + sp(sales)) })
export const useStockStatus = () => useQuery({ queryKey: ['stock', 'status'], refetchInterval: 60_000, queryFn: () => api.get<{ source: string; label: string; synced_at: string | null; every_min: number; live: boolean; can_sync: boolean }>('/stock/status') })
export const useStockPush = (sales?: string) => useQuery({ queryKey: ['stock', 'push', sales ?? 'all'], queryFn: () => api.get<Items<AgingItem>>('/stock/push' + sp(sales)).then((r) => r.items) })
export const useSegmenMoversAll = () => useSegmenMovers()

/** The application's "now": the API clock (ARC_NOW in dev) once known, else the time the page mounted. */
export function useNow(): Date {
  const { data } = useHealth()
  const [mounted] = useState(() => new Date())
  return data ? new Date(data.now) : mounted
}

// ---------- Chat & WhatsApp (stage 03) ----------

/** The conversations of a tab; account is one number's digits, or "team:<sales|admin|billing>" for a team's numbers. */
export const useThreads = (tab = 'all', account = '', enabled = true) =>
  useQuery({
    queryKey: ['chat', 'threads', tab, account],
    enabled,
    queryFn: () => {
      const q = new URLSearchParams()
      if (tab !== 'all') q.set('tab', tab)
      if (account.startsWith('team:')) q.set('team', account.slice(5))
      else if (account) q.set('account', account)
      const qs = q.toString()
      return api.get<Items<ThreadView>>('/chat/threads' + (qs ? '?' + qs : '')).then((r) => r.items)
    },
  })
export const useThread = (id?: string) =>
  useQuery({ queryKey: ['chat', 'thread', id], enabled: !!id, queryFn: () => api.get<ThreadDetail>(`/chat/threads/${id}`) })
export const useChatContext = (id?: string) =>
  useQuery({ queryKey: ['chat', 'context', id], enabled: !!id, queryFn: () => api.get<ChatContext>(`/chat/threads/${id}/context`) })
export const useWAStatus = () =>
  useQuery({ queryKey: ['wa', 'status'], queryFn: () => api.get<{ items: WANumber[]; transport: string }>('/wa/status'), refetchInterval: (q) => (q.state.data?.items.some((n) => n.state === 'pairing') ? 3000 : false) })
export const useInternalNumbers = () => useQuery({ queryKey: ['wa', 'internal'], queryFn: () => api.get<Items<InternalNumber>>('/internal-numbers').then((r) => r.items) })
export const useWAGroups = () => useQuery({ queryKey: ['wa', 'groups'], queryFn: () => api.get<Items<WAGroup>>('/wa/groups').then((r) => r.items) })

export type PolicyRow<T> = { value: T; version: number; updated_at: string }
export const usePolicies = () => useQuery({ queryKey: ['policies'], queryFn: () => api.get<Record<string, PolicyRow<Record<string, unknown>>>>('/policies') })

// ---------- Odoo (stage 04) ----------
export interface SyncState { model: string; last_write_date: string | null; last_run_at: string | null; records: number; error: string | null }
export const useConnections = () =>
  useQuery({ queryKey: ['connections'], queryFn: () => api.get<{ odoo: { mode: string; write: boolean; url: string; models: SyncState[] }; wa: { transport: string; numbers: number; connected: number } }>('/connections') })
export const useOdooCategories = () =>
  useQuery({ queryKey: ['connections', 'categories'], queryFn: () => api.get<{ map: { odoo_category_id: number; kat: string }[]; odoo: { id: number; name: string; complete_name: string }[] | null; kat: string[] }>('/connections/odoo/categories') })

// ---------- Proposals (stage 05) ----------
export const useProposal = (id?: string) => useQuery({ queryKey: ['proposals', 'one', id], enabled: !!id, queryFn: () => api.get<Proposal>(`/proposals/${id}`) })
export const useQueue = (sales?: string) =>
  useQuery({ queryKey: ['proposals', 'queue', sales ?? 'all'], queryFn: () => api.get<Items<Proposal>>('/proposals?queue=1&today=1' + sp(sales, '&')).then((r) => r.items) })

export interface CalibrationAgent { agent: string; confidence: number | null; accepted: number; rejected: number }
export interface CalibrationEvent { id: string; agent: string | null; kind: string | null; decision: string | null; reason: string | null; suppress_until: string | null; created_at: string; title: string | null }
export interface CalibrationLesson { id: string; agent: string; kind: string; reason: string; scope: string; product: string | null; rejections: number; text: string; suppress_until: string | null; created_at: string }
export const useCalibration = () =>
  useQuery({ queryKey: ['proposals', 'calibration'], queryFn: () => api.get<{ agents: CalibrationAgent[]; items: CalibrationEvent[]; lessons: CalibrationLesson[] }>('/calibration') })

export const useStockProposals = () =>
  useQuery({ queryKey: ['proposals', 'stock'], queryFn: () => api.get<Items<Proposal>>('/proposals?agent=' + encodeURIComponent('AI Stok') + '&today=1').then((r) => r.items) })

// ---------- Orchestrator ----------
export const useCycleLatest = () => useQuery({ queryKey: ['cycle', 'latest'], queryFn: () => api.get<CycleLatest>('/cycles/latest'), refetchInterval: 60_000 })
export const useCycleEngines = (enabled = true) =>
  useQuery({ queryKey: ['cycle', 'engines'], enabled, staleTime: 30_000, queryFn: () => api.get<{ model: { ready: boolean; provider: string; model: string; label: string; model_label: string; why: string }; mcp_wait_sec: number }>('/cycles/engines') })
export const useCycles = () => useQuery({ queryKey: ['cycles'], queryFn: () => api.get<Items<Cycle>>('/cycles?limit=100').then((r) => r.items) })
export const useConflicts = (sales?: string, enabled = true) => useQuery({ queryKey: ['cycles', 'conflicts', sales ?? 'all'], enabled, queryFn: () => api.get<Items<Conflict>>('/conflicts' + sp(sales)).then((r) => r.items) })
export const useAgents = (sales?: string) => useQuery({ queryKey: ['agents', sales ?? 'all'], queryFn: () => api.get<Items<AgentInfo>>('/agents' + sp(sales)).then((r) => r.items) })
// Analisa WhatsApp of the last full cycle (ADR 0026); under 'chat' so a new message refreshes the waiting state
export const useWAReport = (days: number, sales?: string) =>
  useQuery({ queryKey: ['wa-report', days, sales ?? 'all'], staleTime: 60_000, refetchInterval: 5 * 60_000, queryFn: () => api.get<WAReport>(`/wa/report?days=${days}` + sp(sales, '&')) })
/** Analisa Dealer by Claude (ADR 0055) and the dealer's WhatsApp conversations (its buttons). */
export const useDealerAnalysis = (id: string) => useQuery({ queryKey: ['dealer', id, 'analysis'], queryFn: () => api.get<{ analysis: DealerAnalysis | null }>(`/dealers/${id}/analysis`).then((r) => r.analysis) })
export const useDealerThreads = (id: string, enabled = true) => useQuery({ queryKey: ['dealer', id, 'threads'], enabled, queryFn: () => api.get<{ items: { id: string; title: string; last_message_at: string | null }[] }>(`/dealers/${id}/threads`).then((r) => r.items) })
export const useWAAnalysis = (sales?: string, enabled = true) => useQuery({ queryKey: ['chat', 'wa-analysis', sales ?? 'all'], queryFn: () => api.get<WAAnalysis>('/wa/analysis' + sp(sales)), enabled })
export const usePlan = (sales?: string) => useQuery({ queryKey: ['plan', 'today', sales ?? 'all'], queryFn: () => api.get<Plan>('/plan/today' + sp(sales)) })
export const useAutonomy = () => useQuery({ queryKey: ['policies', 'autonomy'], queryFn: () => api.get<AutonomyPolicy>('/policies/autonomy') })

// ---------- MCP ----------
export const useMCPInfo = () => useQuery({ queryKey: ['mcp', 'info'], queryFn: () => api.get<MCPInfo>('/mcp/info'), refetchInterval: 30_000 })
// MCP Claude loads itself: a new Claude connection or tool call shows without reloading the page
export const useMCPClients = () => useQuery({ queryKey: ['mcp', 'clients'], queryFn: () => api.get<Items<MCPClient>>('/mcp/clients').then((r) => r.items), refetchInterval: 15_000 })
export const useMCPCalls = (limit = 20) => useQuery({ queryKey: ['mcp', 'calls', limit], queryFn: () => api.get<Items<MCPCall>>(`/mcp/calls?limit=${limit}`).then((r) => r.items), refetchInterval: 10_000 })
export const useAnalyst = () => useQuery({ queryKey: ['mcp', 'analyst'], queryFn: () => api.get<AnalystInfo>('/mcp/analyst') })
export const useSchedules = () => useQuery({ queryKey: ['mcp', 'schedules'], queryFn: () => api.get<Items<Schedule>>('/mcp/schedules').then((r) => r.items) })
export const useScheduleRuns = (id: string) => useQuery({ queryKey: ['mcp', 'runs', id], queryFn: () => api.get<Items<ScheduleRun>>(`/mcp/schedules/${id}/runs?limit=30`).then((r) => r.items) })
export const useScheduleRun = (id: string) => useQuery({ queryKey: ['mcp', 'run', id], queryFn: () => api.get<ScheduleRunFull>(`/mcp/runs/${id}`), enabled: !!id })
export const useCronPreview = (expr: string) => useQuery({ queryKey: ['mcp', 'cron', expr], queryFn: () => api.get<CronPreview>('/mcp/cron?expr=' + encodeURIComponent(expr)), enabled: !!expr.trim(), staleTime: 60_000 })
export const useMCPPolicy = () => useQuery({ queryKey: ['policies', 'mcp'], queryFn: () => api.get<MCPPolicy>('/policies/mcp') })

// ---------- Peta relasi ----------
const rq = (period: number, sales: string) => `?period=${period}${sales !== 'all' ? '&sales=' + encodeURIComponent(sales) : ''}`
export const useRelasi = (period: number, sales: string) => useQuery({ queryKey: ['relasi', period, sales], queryFn: () => api.get<Relasi>('/relasi' + rq(period, sales)) })
export const useRelasiInsights = (period: number, sales: string) =>
  useQuery({ queryKey: ['relasi', 'insights', period, sales], queryFn: () => api.get<Items<RelasiInsight>>('/relasi/insights' + rq(period, sales)).then((r) => r.items) })

// ---------- Push stok · Kredit ----------
export const useStockAging = () => useQuery({ queryKey: ['stock', 'aging'], queryFn: () => api.get<Items<AgingItem>>('/stock/aging').then((r) => r.items) })
export const useStockCritical = () => useQuery({ queryKey: ['stock', 'critical'], queryFn: () => api.get<Items<CriticalItem>>('/stock/critical').then((r) => r.items) })
export const useSalesByProduct = () => useQuery({ queryKey: ['stock', 'sales'], queryFn: () => api.get<Items<ProductSales>>('/stock/sales-by-product?days=30').then((r) => r.items) })
export const useCreditOverview = () => useQuery({ queryKey: ['credit', 'overview'], queryFn: () => api.get<CreditOverview>('/credit/overview') })
export const useCreditDealers = () => useQuery({ queryKey: ['credit', 'dealers'], queryFn: () => api.get<Items<ARRow>>('/credit/dealers').then((r) => r.items) })
export const useCreditExposure = () => useQuery({ queryKey: ['credit', 'exposure'], queryFn: () => api.get<Items<ExposureRow>>('/credit/exposure').then((r) => r.items) })
export const useScreenAnalysis = (screen: 'stock' | 'credit' | 'orbit' | 'segmen' | 'relasi') => useQuery({ queryKey: [screen, 'analysis'], queryFn: () => api.get<{ analysis: ScreenAnalysis | null }>('/screen-analysis/' + screen).then((r) => r.analysis) })
export const useCreditForecast = () => useQuery({ queryKey: ['credit', 'forecast'], queryFn: () => api.get<Items<ForecastRow>>('/credit/forecast?days=30').then((r) => r.items) })
