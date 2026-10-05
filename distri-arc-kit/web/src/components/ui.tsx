import type { CSSProperties, ReactNode } from 'react'
import { Icon } from './Icon'
import { hcol } from '../lib/format'

export type Tone = 'good' | 'warn' | 'bad' | 'accent' | 'indigo' | 'neutral'

export function Pill({ tone, icon, children, style, className = '' }: { tone: Tone; icon?: string; children: ReactNode; style?: CSSProperties; className?: string }) {
  return (
    <span className={`pill ${tone} ${className}`.trim()} style={style}>
      {icon && <Icon name={icon} />}
      {children}
    </span>
  )
}

/** The gradient AI label ("disusun Orchestrator", "AI Follow-up"). */
export function Ai({ children, style }: { children?: ReactNode; style?: CSSProperties }) {
  return <span className="ai" style={style}>{children}</span>
}

export function Prov({ icon, children, style }: { icon?: string; children: ReactNode; style?: CSSProperties }) {
  return (
    <span className="prov" style={style}>
      {icon && <Icon name={icon} />}
      {children}
    </span>
  )
}

/** Score ring (56 px) exactly as the mockup ringSvg. */
export function ScoreRing({ h }: { h: number }) {
  const c = 2 * Math.PI * 24
  return (
    <svg className="ring" viewBox="0 0 56 56">
      <circle className="track" cx="28" cy="28" r="24" />
      <circle cx="28" cy="28" r="24" stroke={hcol(h)} strokeDasharray={c} strokeDashoffset={c * (1 - h / 100)} transform="rotate(-90 28 28)" />
      <text x="28" y="33" textAnchor="middle">{h}</text>
    </svg>
  )
}

export function CardH({ title, ai, meta, children }: { title?: ReactNode; ai?: ReactNode; meta?: ReactNode; children?: ReactNode }) {
  return (
    <div className="card-h">
      {title && <h2>{title}</h2>}
      {ai && <span className="ai" style={title ? { marginLeft: 6 } : undefined}>{ai}</span>}
      {meta !== undefined && <span className="meta">{meta}</span>}
      {children}
    </div>
  )
}

/** Dealer name link (".ev" dotted underline) that opens the dealer page. */
export function DealerLink({ id, children, onGo }: { id: string; children: ReactNode; onGo: (id: string) => void }) {
  return (
    <button className="ev" onClick={() => onGo(id)}>
      {children}
    </button>
  )
}
