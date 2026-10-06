import { useEffect, useState } from 'react'

// Appearance per device (not a policy): theme light by default, and the sidebar colour (mejikuhibiniu). Applied as
// data-theme / data-rail on <html>; tokens.css and app.css do the rest.

export type Theme = 'light' | 'dark' | 'auto'
export type RailColor = 'netral' | 'merah' | 'jingga' | 'kuning' | 'hijau' | 'biru' | 'nila' | 'ungu'

export const THEMES: [Theme, string][] = [['light', 'Terang'], ['dark', 'Gelap'], ['auto', 'Otomatis']]
export const RAIL_COLORS: [RailColor, string, string][] = [
  ['netral', 'Netral', 'var(--bg)'],
  ['merah', 'Merah', '#D93025'],
  ['jingga', 'Jingga', '#E8590C'],
  ['kuning', 'Kuning', '#F2C200'],
  ['hijau', 'Hijau', '#1E9E4A'],
  ['biru', 'Biru', '#0071E3'],
  ['nila', 'Nila', '#3F3DB8'],
  ['ungu', 'Ungu', '#8E3BD9'],
]

const KEY_THEME = 'arc.theme'
const KEY_RAIL = 'arc.rail'
const EVENT = 'arc-appearance'

function read<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  try {
    const v = localStorage.getItem(key) as T | null
    return v && allowed.includes(v) ? v : fallback
  } catch {
    return fallback
  }
}

export const storedTheme = () => read<Theme>(KEY_THEME, ['light', 'dark', 'auto'], 'light')
export const storedRail = () => read<RailColor>(KEY_RAIL, RAIL_COLORS.map((c) => c[0]), 'netral')

/** Applies the stored appearance to <html> (before the first render and after every change). */
export function applyAppearance(theme = storedTheme(), rail = storedRail()) {
  const el = document.documentElement
  if (theme === 'auto') delete el.dataset.theme
  else el.dataset.theme = theme
  if (rail === 'netral') delete el.dataset.rail
  else el.dataset.rail = rail
}

function save(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // private mode / storage blocked: the choice lasts for this page only
  }
}

/** Current appearance with setters; every open component follows the change. */
export function useAppearance() {
  const [theme, setThemeState] = useState<Theme>(storedTheme)
  const [rail, setRailState] = useState<RailColor>(storedRail)
  useEffect(() => {
    const sync = () => {
      setThemeState(storedTheme())
      setRailState(storedRail())
    }
    window.addEventListener(EVENT, sync)
    window.addEventListener('storage', sync)
    return () => {
      window.removeEventListener(EVENT, sync)
      window.removeEventListener('storage', sync)
    }
  }, [])
  const setTheme = (t: Theme) => {
    save(KEY_THEME, t)
    applyAppearance(t, storedRail())
    window.dispatchEvent(new Event(EVENT))
  }
  const setRail = (r: RailColor) => {
    save(KEY_RAIL, r)
    applyAppearance(storedTheme(), r)
    window.dispatchEvent(new Event(EVENT))
  }
  return { theme, rail, setTheme, setRail }
}
