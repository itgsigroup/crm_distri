import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import type { Action, Me } from '../api/types'
import { Ctx, parseHash, type Route, type UIState } from './ui'

// UIProvider holds routing (hash), toast, the global action sheet, the refresh
// counter, the signed-in user, and a pending Ask question.
export function UIProvider({ children }: { children: ReactNode }) {
  const [route, setRoute] = useState<Route>(parseHash)
  const [toastMsg, setToastMsg] = useState<string | null>(null)
  const [sheet, setSheet] = useState<Action | string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)
  const [me, setMe] = useState<Me | null>(null)
  const [pendingAsk, setPendingAsk] = useState<string | null>(null)

  useEffect(() => {
    const on = () => setRoute(parseHash())
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])

  const go = useCallback((target: string) => {
    const [screen, param] = target.split(':')
    const next = '#' + screen + (param ? '/' + param : '')
    if (window.location.hash !== next) window.location.hash = next
    else setRoute(parseHash())
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  const toast = useCallback((msg: string) => {
    setToastMsg(msg)
  }, [])
  useEffect(() => {
    if (!toastMsg) return
    const t = setTimeout(() => setToastMsg(null), 2600)
    return () => clearTimeout(t)
  }, [toastMsg])

  const refresh = useCallback(() => setRefreshKey(k => k + 1), [])
  const ask = useCallback((q: string) => {
    setPendingAsk(q)
    go('ask')
  }, [go])
  const consumeAsk = useCallback(() => {
    const q = pendingAsk
    setPendingAsk(null)
    return q
  }, [pendingAsk])

  const value = useMemo<UIState>(() => ({
    route, go, toast, toastMsg, sheet,
    openSheet: a => setSheet(a), closeSheet: () => setSheet(null),
    refreshKey, refresh, me, setMe, pendingAsk, ask, consumeAsk,
  }), [route, go, toast, toastMsg, sheet, refreshKey, refresh, me, pendingAsk, ask, consumeAsk])

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}
