import { createContext, useContext } from 'react'
import type { Action, Me, ScreenKey } from '../api/types'

export const SCREENS: ScreenKey[] = ['today', 'ask', 'chat', 'rel', 'net', 'pipe', 'pros', 'cash', 'conn']

export interface Route { screen: ScreenKey; param: string }

export interface UIState {
  route: Route
  go: (target: string) => void          // "rel:rsud", "pros", "conn:sec-wa"
  toast: (msg: string) => void
  toastMsg: string | null
  sheet: Action | string | null
  openSheet: (a: Action | string) => void
  closeSheet: () => void
  refreshKey: number
  refresh: () => void
  me: Me | null
  setMe: (m: Me | null) => void
  pendingAsk: string | null
  ask: (q: string) => void              // navigates to Ask and submits q
  consumeAsk: () => string | null
}

export const Ctx = createContext<UIState | null>(null)

export function parseHash(): Route {
  const h = window.location.hash.replace(/^#\/?/, '')
  const [screen, ...rest] = h.split('/')
  if ((SCREENS as string[]).includes(screen)) return { screen: screen as ScreenKey, param: rest.join('/') }
  return { screen: 'today', param: '' }
}

export function useUI(): UIState {
  const v = useContext(Ctx)
  if (!v) throw new Error('useUI outside UIProvider')
  return v
}
