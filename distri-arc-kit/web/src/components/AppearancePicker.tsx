import { RAIL_COLORS, THEMES, useAppearance } from '../app/appearance'

/** Tema (Terang / Gelap / Otomatis) + warna sidebar mejikuhibiniu. Used in the account menu and Pengaturan. */
export function AppearancePicker({ compact = false }: { compact?: boolean }) {
  const { theme, rail, setTheme, setRail } = useAppearance()
  return (
    <div className={`appearance ${compact ? 'compact' : ''}`}>
      <span className="ap-label">Tema</span>
      <div className="seg" role="radiogroup" aria-label="Tema">
        {THEMES.map(([k, l]) => <button key={k} role="radio" aria-checked={theme === k} className={theme === k ? 'is-active' : ''} onClick={() => setTheme(k)}>{l}</button>)}
      </div>
      <span className="ap-label">Warna sidebar</span>
      <div className="swatches" role="radiogroup" aria-label="Warna sidebar">
        {RAIL_COLORS.map(([k, l, c]) => (
          <button key={k} role="radio" aria-checked={rail === k} aria-label={l} title={l} className={`swatch ${rail === k ? 'is-active' : ''} ${k === 'netral' ? 'neutral' : ''}`} style={{ background: c }} onClick={() => setRail(k)} />
        ))}
      </div>
    </div>
  )
}
