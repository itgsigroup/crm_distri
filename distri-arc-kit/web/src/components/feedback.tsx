import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon } from './Icon'

// Toast and Sheet are single, app-wide elements in the mockup (#toast, #sheet); this provider owns both.

interface Feedback {
  toast: (msg: string) => void
  openSheet: (content: ReactNode) => void
  closeSheet: () => void
}

const Ctx = createContext<Feedback | null>(null)

export function useFeedback() {
  const v = useContext(Ctx)
  if (!v) throw new Error('FeedbackProvider missing')
  return v
}

export function FeedbackProvider({ children }: { children: ReactNode }) {
  const [msg, setMsg] = useState('')
  const [show, setShow] = useState(false)
  const [sheet, setSheet] = useState<ReactNode>(null)
  const [sheetOpen, setSheetOpen] = useState(false)
  const timer = useRef<number | undefined>(undefined)

  const toast = useCallback((m: string) => {
    setMsg(m)
    setShow(true)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setShow(false), 2600)
  }, [])
  const openSheet = useCallback((c: ReactNode) => {
    setSheet(c)
    setSheetOpen(true)
  }, [])
  const closeSheet = useCallback(() => setSheetOpen(false), [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setSheetOpen(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  return (
    <Ctx.Provider value={{ toast, openSheet, closeSheet }}>
      {children}
      <div className={`sheet-bg ${sheetOpen ? 'show' : ''}`} onClick={closeSheet} />
      <div className={`sheet ${sheetOpen ? 'show' : ''}`} role="dialog" aria-modal="true" aria-labelledby="sheet-title">
        {sheet}
      </div>
      <div className={`toast ${show ? 'show' : ''}`} role="status" aria-live="polite">
        <Icon name="check" />
        {msg}
      </div>
    </Ctx.Provider>
  )
}

/** Sheet header used by every sheet ("sh"). */
export function SheetHead({ icon, title, sub, onClose }: { icon: string; title: ReactNode; sub?: ReactNode; onClose: () => void }) {
  return (
    <div className="sh">
      <span className="ni"><Icon name={icon} /></span>
      <div>
        <h3 id="sheet-title">{title}</h3>
        {sub && <small>{sub}</small>}
      </div>
      <button className="close" onClick={onClose} aria-label="Tutup"><Icon name="x" /></button>
    </div>
  )
}
