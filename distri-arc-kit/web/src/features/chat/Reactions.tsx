import { useEffect, useRef, useState } from 'react'
import type { MessageView } from '../../api/types'
import './reactions.css'

export type Reaction = NonNullable<MessageView['reactions']>[number]

/** The six WhatsApp offers first, then more behind "+". */
const QUICK = ['👍', '❤️', '😂', '😮', '😢', '🙏']
const MORE = ['😀', '😁', '😊', '😍', '🥰', '😘', '😎', '🤩', '🤔', '😅', '🤣', '😭', '😡', '😱', '🥳', '😴',
  '👏', '🙌', '👌', '✌️', '🤝', '💪', '👀', '🔥', '✨', '🎉', '💯', '✅', '❌', '⚠️', '📦', '🚚',
  '💰', '📌', '⏰', '👎', '🤦', '🤷', '💙', '💚']

export function ReactIcon() {
  return (
    <svg className="i" viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="2" />
      <circle cx="9" cy="10" r="1.3" fill="currentColor" /><circle cx="15" cy="10" r="1.3" fill="currentColor" />
      <path d="M8 14.2c1 1.5 2.4 2.3 4 2.3s3-.8 4-2.3" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}

/** The emoji row over a bubble (WhatsApp "Reaksi"): picking the one already given takes it back. */
export function ReactionPicker({ mine, side, onPick, onClose }: { mine?: string; side: 'in' | 'out'; onPick: (emoji: string) => void; onClose: () => void }) {
  const [more, setMore] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const away = (e: MouseEvent) => { if (box.current && !box.current.contains(e.target as Node)) onClose() }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('mousedown', away)
    document.addEventListener('keydown', esc)
    return () => { document.removeEventListener('mousedown', away); document.removeEventListener('keydown', esc) }
  }, [onClose])
  const pick = (e: string) => { onPick(e === mine ? '' : e); onClose() }
  const btn = (e: string) => (
    <button key={e} type="button" className={e === mine ? 'is-mine' : undefined} onClick={() => pick(e)} aria-label={e === mine ? `Batalkan reaksi ${e}` : `Reaksi ${e}`}>{e}</button>
  )
  return (
    <div ref={box} className={`rx-pick rx-${side}`} role="dialog" aria-label="Pilih reaksi">
      <div className="rx-quick">
        {QUICK.map(btn)}
        <button type="button" className="rx-plus" onClick={() => setMore((v) => !v)} aria-expanded={more} aria-label="Emoji lain">+</button>
      </div>
      {more && <div className="rx-more">{MORE.map(btn)}</div>}
    </div>
  )
}

/** The reactions under a bubble, one chip per emoji; the number's own is marked and a click on it takes it back. */
export function ReactionChips({ rs, onPick }: { rs: Reaction[]; onPick?: (emoji: string) => void }) {
  const mine = rs.find((r) => r.mine)?.emoji
  return (
    <div className="rx-chips">
      {rs.map((r) => (
        <button key={r.emoji} type="button" className={`rx-chip${r.mine ? ' is-mine' : ''}`} disabled={!onPick}
          onClick={() => onPick?.(r.emoji === mine ? '' : r.emoji)}
          title={r.mine ? (r.count > 1 ? `Anda dan ${r.count - 1} lainnya · klik untuk batalkan` : 'Reaksi Anda · klik untuk batalkan') : `${r.count} orang · klik untuk ikut`}>
          {r.emoji}{r.count > 1 && <span>{r.count}</span>}
        </button>
      ))}
    </div>
  )
}

/** The reactions after this number reacts with emoji ("" takes its reaction back): shown at once, before WhatsApp. */
export function withMine(rs: Reaction[] | undefined, emoji: string): Reaction[] {
  const out = (rs ?? []).map((r) => (r.mine ? { ...r, count: r.count - 1, mine: false } : r)).filter((r) => r.count > 0)
  if (!emoji) return out
  const at = out.findIndex((r) => r.emoji === emoji)
  if (at >= 0) out[at] = { ...out[at], count: out[at].count + 1, mine: true }
  else out.push({ emoji, count: 1, mine: true })
  return out
}
