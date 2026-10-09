import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon } from './Icon'

// Toast and Sheet are single, app-wide elements in the mockup (#toast, #sheet); this provider owns both.

/** A SweetAlert-style dialog: big animated icon, title, text, optional detail rows, one OK button. */
export interface AlertSpec {
  icon: 'success' | 'error' | 'warning'
  title: string
  text?: string
  details?: [string, string][]
}

interface Feedback {
  toast: (msg: string) => void
  alert: (spec: AlertSpec) => void
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
  const [dialog, setDialog] = useState<AlertSpec | null>(null)
  const okRef = useRef<HTMLButtonElement>(null)
  const alert = useCallback((spec: AlertSpec) => setDialog(spec), [])
  useEffect(() => { if (dialog) okRef.current?.focus() }, [dialog])

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
      if (e.key === 'Escape') {
        setSheetOpen(false)
        setDialog(null)
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  return (
    <Ctx.Provider value={{ toast, alert, openSheet, closeSheet }}>
      {children}
      <div className={`sheet-bg ${sheetOpen ? 'show' : ''}`} onClick={closeSheet} />
      <div className={`sheet ${sheetOpen ? 'show' : ''}`} role="dialog" aria-modal="true" aria-labelledby="sheet-title">
        {sheet}
      </div>
      {dialog && (
        <div className="swal-bg" onClick={() => setDialog(null)}>
          <div className={`swal swal-${dialog.icon}`} role="alertdialog" aria-modal="true" aria-labelledby="swal-title" aria-describedby="swal-text" onClick={(e) => e.stopPropagation()}>
            <div className="swal-icon" aria-hidden="true">
              {dialog.icon === 'success' ? (
                <svg viewBox="0 0 52 52"><circle className="swal-ring" cx="26" cy="26" r="24" /><path className="swal-mark" d="M15 27l7 7 15-16" /></svg>
              ) : dialog.icon === 'error' ? (
                <svg viewBox="0 0 52 52"><circle className="swal-ring" cx="26" cy="26" r="24" /><path className="swal-mark" d="M18 18l16 16M34 18L18 34" /></svg>
              ) : (
                <svg viewBox="0 0 52 52"><circle className="swal-ring" cx="26" cy="26" r="24" /><path className="swal-mark" d="M26 14v16M26 37v1" /></svg>
              )}
            </div>
            <h3 id="swal-title">{dialog.title}</h3>
            {dialog.text && <p id="swal-text">{dialog.text}</p>}
            {!!dialog.details?.length && (
              <dl className="swal-details">{dialog.details.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
            )}
            <button ref={okRef} type="button" className="btn primary swal-ok" onClick={() => setDialog(null)}>OK</button>
          </div>
        </div>
      )}
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
