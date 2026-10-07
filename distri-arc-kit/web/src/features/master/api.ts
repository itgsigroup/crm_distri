import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'

export interface UserRow {
  id: string; email: string; name: string; role: string; role_key: string; role_name: string; wa_allowed: boolean; active: boolean; has_password: boolean
  sales_name: string | null; branch: string | null; wa_number: string | null; wa_state: string | null; wa_linked: string | null
}
export interface RoleRow {
  key: string; name: string; description: string; base: string; active: boolean; wa_allowed: boolean; users: number
  screens: string[]; decide: string[]; scope: 'all' | 'own'; policies: boolean
}
export interface RoleCatalog {
  items: RoleRow[]
  screens: { key: string; label: string; group: string; all_data: boolean }[]
  kinds: { key: string; label: string; policies: boolean }[]
}
export interface Branch { id: string; name: string; city: string; address: string; active: boolean; users: number; dealers: number }

export const useUsers = (enabled = true) => useQuery({ queryKey: ['users'], queryFn: () => api.get<{ items: UserRow[] }>('/users').then((r) => r.items), enabled })
export const useRoles = (enabled = true) => useQuery({ queryKey: ['roles'], queryFn: () => api.get<RoleCatalog>('/roles'), enabled })
export const useBranchMaster = () => useQuery({ queryKey: ['branches'], queryFn: () => api.get<{ items: Branch[] }>('/branches').then((r) => r.items) })

export const fmtWA = (n: string | null | undefined) => (n ? '+' + n.replace(/^(\d{2})(\d{3})(\d{4})(\d+)$/, '$1 $2-$3-$4') : '')
export const field = { width: '100%', height: 36, borderRadius: 9, border: '1px solid var(--line)', padding: '0 10px', font: 'inherit', fontSize: 14, background: 'var(--surface)', color: 'var(--text)', marginTop: 4 }
