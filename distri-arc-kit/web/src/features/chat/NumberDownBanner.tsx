import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import type { WANumber } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { hhmm, shortDate } from '../../lib/format'
import { useMe, useWAStatus } from '../../app/queries'
import { labelOf, PairSheet } from './Connect'
import './numberdown.css'

const since = (n: WANumber) => {
  const at = n.state_since ?? n.last_seen_at
  return at ? ` sejak ${shortDate(at)} ${hhmm(at)}` : ''
}

/** Numbers WhatsApp removed from their linked device: they sync nothing until a person links them again. */
export const loggedOut = (items: WANumber[] | undefined) => (items ?? []).filter((n) => n.state === 'logged_out')

/** One logout of one number: a later logout of the same number (after linking again) pops up again. */
const eventKey = (n: WANumber) => `wa-logout-seen:${n.wa_number}:${n.state_since ?? n.last_seen_at ?? ''}`

function seen(n: WANumber) {
  try { return sessionStorage.getItem(eventKey(n)) === '1' } catch { return false }
}

function markSeen(ns: WANumber[]) {
  try { ns.forEach((n) => sessionStorage.setItem(eventKey(n), '1')) } catch { /* private mode: the popup shows again */ }
}

/** A SweetAlert-style popup the moment a number is logged out (Sam 27 Oct): it has to be linked again. Shown once per
 * logout per browser session; the banner below stays until the number is linked. */
function LoggedOutPopup({ down, own }: { down: WANumber[]; own: boolean }) {
  const { openSheet } = useFeedback()
  const nav = useNavigate()
  const [, rerender] = useState(0)
  const btn = useRef<HTMLButtonElement>(null)
  const open = down.filter((n) => !seen(n))
  useEffect(() => { if (open.length) btn.current?.focus() }, [open.length])
  if (!open.length) return null
  const close = () => { markSeen(open); rerender((x) => x + 1) }
  const one = open.length === 1 ? open[0] : undefined
  const relink = () => {
    close()
    if (one) openSheet(<PairSheet wa={one.wa_number} />)
    else nav('/chat')
  }
  return (
    <div className="swal-bg" onClick={close}>
      <div className="swal swal-error wa-logout-swal" role="alertdialog" aria-modal="true" aria-labelledby="wa-logout-title" aria-describedby="wa-logout-text" onClick={(e) => e.stopPropagation()} onKeyDown={(e) => { if (e.key === 'Escape') close() }}>
        <div className="swal-icon" aria-hidden="true">
          <svg viewBox="0 0 52 52"><circle className="swal-ring" cx="26" cy="26" r="24" /><path className="swal-mark" d="M26 14v16M26 37v1" /></svg>
        </div>
        <h3 id="wa-logout-title">WhatsApp logout</h3>
        <p id="wa-logout-text">
          {one
            ? <>{own ? 'WhatsApp Anda' : 'WhatsApp'} <b>{labelOf(one)}</b> keluar dari perangkat tertaut{since(one)}. </>
            : <>{open.length} nomor WhatsApp keluar dari perangkat tertaut. </>}
          Riwayat chat dikunci dan chat baru tidak masuk. <b>Nomor harus ditautkan ulang</b> dengan scan QR dari HP.
        </p>
        {!one && <dl className="swal-details">{open.map((n) => <div key={n.wa_number}><dt>{labelOf(n)}</dt><dd>{since(n).trim() || 'logout'}</dd></div>)}</dl>}
        <div className="wa-logout-actions">
          <button type="button" className="btn ghost" onClick={close}>Nanti</button>
          <button ref={btn} type="button" className="btn primary swal-ok" onClick={relink}><Icon name="qr" />{one ? 'Tautkan ulang' : 'Buka Chat'}</button>
        </div>
      </div>
    </div>
  )
}

/** On every page, the numbers that need linking again (Sam 27 Oct: Granike's number logged out and nobody noticed).
 * A holder sees their own number with "Tautkan ulang"; CEO/admin see every logged-out number. The first time a logout
 * is seen, a popup asks to link it again. */
export function NumberDownBanner() {
  const { data: me } = useMe()
  const { data: wa } = useWAStatus()
  const { openSheet } = useFeedback()
  const nav = useNavigate()
  if (!me?.screens.includes('chat')) return null
  const own = me.chat_scope === 'own'
  const down = loggedOut(wa?.items).filter((n) => !own || n.mine)
  if (!down.length) return null
  const one = down.length === 1 ? down[0] : undefined
  const text = one
    ? `${own ? 'WhatsApp Anda' : 'WhatsApp'} ${labelOf(one)} logout dari GSI Orbit${since(one)}. Chat baru tidak masuk dan riwayat chat terkunci sampai nomor ditautkan ulang.`
    : `${down.length} nomor WhatsApp logout dari GSI Orbit: ${down.map(labelOf).join(', ')}. Chat baru tidak masuk dan riwayat chatnya terkunci sampai ditautkan ulang.`
  return (
    <>
      <div className="wa-down-banner" role="alert">
        <Icon name="alert" /><span>{text}</span>
        {one
          ? <button type="button" className="btn" onClick={() => openSheet(<PairSheet wa={one.wa_number} />)}><Icon name="qr" />Tautkan ulang</button>
          : <button type="button" className="btn" onClick={() => nav('/chat')}><Icon name="qr" />Buka Chat</button>}
      </div>
      <LoggedOutPopup down={down} own={own} />
    </>
  )
}

/** Chat of a number that is logged out: the history stays locked for everyone until it is linked again (the server
 * refuses its conversations with 423). */
export function NumberLocked({ n }: { n: WANumber }) {
  const { openSheet } = useFeedback()
  return (
    <div className="wa-locked">
      <Icon name="lock" />
      <b>Riwayat chat terkunci</b>
      <span>WhatsApp {labelOf(n)} keluar dari perangkat tertaut{since(n)}. Tautkan ulang nomor ini dari HP untuk membuka riwayat chat dan menerima chat baru.</span>
      <button type="button" className="btn primary" onClick={() => openSheet(<PairSheet wa={n.wa_number} />)}><Icon name="qr" />Tautkan ulang</button>
    </div>
  )
}
