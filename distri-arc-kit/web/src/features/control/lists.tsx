import { useState } from 'react'
import { useNavigate } from 'react-router'
import type { Brief } from '../../api/types'
import { PlanText } from './Plan'
import { Icon } from '../../components/Icon'

export const creditTone = (s: string) => (s === 'aman' ? 'good' : s === 'tipis' ? 'warn' : s === 'cash' ? 'neutral' : 'bad')

export function useGoDealer() {
  const nav = useNavigate()
  return (id: string) => nav('/dealer/' + id)
}

export const dueLabel = (n: number) => (n === 0 ? 'hari ini' : n === 1 ? 'besok' : `${n} hr`)

/** "Ringkasan Orchestrator" points (mockup renderBrief), written from the structured brief. */
const BRIEF_ICON: Record<string, [string, string]> = { on_schedule: ['good', 'check'], drift: ['warn', 'refresh'], credit: ['bad', 'shield'], push: ['accent', 'box'] }

/** Ringkasan Orchestrator (mockup renderBrief): four points written by the cycle, dealer names link to the dealer. */
export function BriefPoints({ brief }: { brief: Brief }) {
  const [open, setOpen] = useState<string[]>([])
  return (
    <ol>
      {brief.points.map((p) => {
        const [k, icon] = BRIEF_ICON[p.kind] ?? ['accent', 'spark']
        const long = p.text.length > 600 // a point listing dozens of dealers: 4 lines, then "Selengkapnya"
        const isOpen = open.includes(p.kind)
        return (
          <li key={p.kind} title={p.signal_ids.length ? `${p.signal_ids.length} sumber` : undefined}>
            <span className={`k ${k}`}><Icon name={icon} /></span>
            <div>
              <div className={long && !isOpen ? 'bp-clamp' : undefined}><b>{p.title}</b>: <PlanText text={p.text} /></div>
              {long && <button className="ev-more bp-toggle" onClick={() => setOpen(isOpen ? open.filter((x) => x !== p.kind) : [...open, p.kind])}>{isOpen ? 'Ringkas' : 'Selengkapnya'}</button>}
            </div>
          </li>
        )
      })}
    </ol>
  )
}
