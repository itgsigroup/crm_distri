import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type Items } from '../api/client'
import type { Proposal } from '../api/types'
import type { ChatContext, InternalNumber, ThreadDetail, ThreadView, WAGroup, WANumber } from '../api/types'
import type {
  AgendaRow, AgingItem, BoardItem, Brief, DealerDetail, Health, KPI, Me, Mover, Sales, SegmentSummary, StatusSummary,
} from '../api/types'

// Query keys follow docs/design/08-frontend.md (Data & realtime); SSE events invalidate them.

const q = (sales?: string) => (sales && sales !== 'all' ? `?sales=${encodeURIComponent(sales)}` : '')

export const useMe = () => useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me'), staleTime: 60_000 })
export const useHealth = () => useQuery({ queryKey: ['health'], queryFn: () => api.get<Health>('/health'), staleTime: 30_000 })
export const useSales = () => useQuery({ queryKey: ['sales'], queryFn: () => api.get<Items<Sales>>('/sales').then((r) => r.items) })
export const useDealers = (term = '') =>
  useQuery({ queryKey: ['dealers', term], queryFn: () => api.get<Items<BoardItem>>('/dealers' + (term ? `?q=${encodeURIComponent(term)}` : '')).then((r) => r.items) })
export const useDealer = (id: string | undefined) =>
  useQuery({ queryKey: ['dealer', id], enabled: !!id, queryFn: () => api.get<DealerDetail>(`/dealers/${id}`) })
export const useOrbit = (sales?: string) => useQuery({ queryKey: ['orbit', sales ?? 'all'], queryFn: () => api.get<Items<BoardItem>>('/orbit' + q(sales)).then((r) => r.items) })
export const useOrbitSummary = (sales?: string) =>
  useQuery({ queryKey: ['orbit', 'summary', sales ?? 'all'], queryFn: () => api.get<Items<StatusSummary>>('/orbit/summary' + q(sales)).then((r) => r.items) })
export const useOrbitMovers = (sales?: string) =>
  useQuery({ queryKey: ['orbit', 'movers', sales ?? 'all'], queryFn: () => api.get<Items<Mover>>('/orbit/movers' + q(sales)).then((r) => r.items) })
export const useSegmen = (sales?: string) =>
  useQuery({ queryKey: ['segmen', sales ?? 'all'], queryFn: () => api.get<{ items: BoardItem[]; thresholds: { freq_per_month: number; size_idr: number } }>('/segmen' + q(sales)) })
export const useSegmenSummary = (sales?: string) =>
  useQuery({ queryKey: ['segmen', 'summary', sales ?? 'all'], queryFn: () => api.get<{ items: SegmentSummary[]; total_omzet_bln: number }>('/segmen/summary' + q(sales)) })
export const useSegmenMovers = (sales?: string) =>
  useQuery({ queryKey: ['segmen', 'movers', sales ?? 'all'], queryFn: () => api.get<Items<Mover>>('/segmen/movers' + q(sales)).then((r) => r.items) })
export const useDue = (days = 7) => useQuery({ queryKey: ['dealers', 'due', days], queryFn: () => api.get<Items<BoardItem>>(`/dealers/due?days=${days}`).then((r) => r.items) })
export const useDrift = () => useQuery({ queryKey: ['dealers', 'drift'], queryFn: () => api.get<Items<BoardItem>>('/dealers/drift').then((r) => r.items) })
export const useCreditTight = () => useQuery({ queryKey: ['dealers', 'credit-tight'], queryFn: () => api.get<Items<BoardItem>>('/dealers/credit-tight').then((r) => r.items) })
export const useKpi = () => useQuery({ queryKey: ['kpi'], queryFn: () => api.get<KPI>('/kpi') })
export const useAgenda = () => useQuery({ queryKey: ['agenda'], queryFn: () => api.get<Items<AgendaRow>>('/agenda').then((r) => r.items) })
export const useBrief = () => useQuery({ queryKey: ['brief', 'today'], queryFn: () => api.get<Brief>('/brief/today') })
export const useStockPush = () => useQuery({ queryKey: ['stock', 'push'], queryFn: () => api.get<Items<AgingItem>>('/stock/push').then((r) => r.items) })
export const useSegmenMoversAll = () => useSegmenMovers()

/** The application's "now": the API clock (ARC_NOW in dev) once known, else the time the page mounted. */
export function useNow(): Date {
  const { data } = useHealth()
  const [mounted] = useState(() => new Date())
  return data ? new Date(data.now) : mounted
}

// ---------- Chat & WhatsApp (stage 03) ----------

export const useThreads = (tab = 'all') =>
  useQuery({ queryKey: ['chat', 'threads', tab], queryFn: () => api.get<Items<ThreadView>>('/chat/threads' + (tab === 'all' ? '' : `?tab=${tab}`)).then((r) => r.items) })
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
export const useQueue = () =>
  useQuery({ queryKey: ['proposals', 'queue'], queryFn: () => api.get<Items<Proposal>>('/proposals?queue=1&today=1').then((r) => r.items) })

export interface CalibrationAgent { agent: string; confidence: number | null; accepted: number; rejected: number }
export interface CalibrationEvent { id: string; agent: string | null; kind: string | null; decision: string | null; reason: string | null; suppress_until: string | null; created_at: string; title: string | null }
export const useCalibration = () =>
  useQuery({ queryKey: ['proposals', 'calibration'], queryFn: () => api.get<{ agents: CalibrationAgent[]; items: CalibrationEvent[] }>('/calibration') })
