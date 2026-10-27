import { useNavigate } from 'react-router'
import type { WANumber } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { hhmm, shortDate } from '../../lib/format'
import { useMe, useWAStatus } from '../../app/queries'
import { labelOf, PairSheet } from './Connect'

const since = (n: WANumber) => (n.last_seen_at ? ` sejak ${shortDate(n.last_seen_at)} ${hhmm(n.last_seen_at)}` : '')

/** Numbers WhatsApp removed from their linked device: they sync nothing until a person links them again. */
export const loggedOut = (items: WANumber[] | undefined) => (items ?? []).filter((n) => n.state === 'logged_out')

/** On every page, the numbers that need linking again (Sam 27 Oct: Granike's number logged out and nobody noticed).
 * A holder sees their own number with "Tautkan ulang"; CEO/admin see every logged-out number. */
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
    ? own
      ? `WhatsApp Anda (${labelOf(one)}) terputus dari GSI Orbit${since(one)}. Chat baru tidak masuk dan riwayat chat terkunci sampai nomor ditautkan ulang.`
      : `WhatsApp ${labelOf(one)} terputus dari GSI Orbit${since(one)} — chat baru tidak masuk sampai ditautkan ulang.`
    : `${down.length} nomor WhatsApp terputus dari GSI Orbit: ${down.map(labelOf).join(', ')} — chat baru tidak masuk sampai ditautkan ulang.`
  return (
    <div className="wa-down-banner" role="alert">
      <Icon name="alert" /><span>{text}</span>
      {one
        ? <button type="button" className="btn" onClick={() => openSheet(<PairSheet wa={one.wa_number} />)}><Icon name="qr" />Tautkan ulang</button>
        : <button type="button" className="btn" onClick={() => nav('/chat')}><Icon name="qr" />Buka Chat</button>}
    </div>
  )
}

/** Chat of a number that is logged out, for a viewer of their own numbers: the history stays locked until it is
 * linked again (the server refuses its conversations with 423). */
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
