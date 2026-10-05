import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type Items } from '../api/client'
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
