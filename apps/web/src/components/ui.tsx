// Small shared primitives ported from the mockup's helper functions (ic, ring, stars, pill).
import type { CSSProperties, ReactNode } from 'react'
import type { Pill as PillT } from '../api/types'
import { hcol } from '../lib/format'

export function Icon({ n, cls = 'i', style }: { n: string; cls?: string; style?: CSSProperties }) {
  return (
    <svg className={cls} style={style} aria-hidden="true">
      <use href={'#' + n} />
    </svg>
  )
}

export function Ring({ h }: { h: number }) {
  const c = 2 * Math.PI * 24
  return (
    <svg className="ring" viewBox="0 0 56 56" role="img" aria-label={'Health ' + h}>
      <circle className="track" cx="28" cy="28" r="24" />
      <circle cx="28" cy="28" r="24" stroke={hcol(h)} strokeDasharray={c} strokeDashoffset={c * (1 - h / 100)} transform="rotate(-90 28 28)" />
      <text x="28" y="33" textAnchor="middle">{h}</text>
    </svg>
  )
}

export function Stars({ n }: { n: number }) {
  const v = Math.max(n, 0)
  return <span className={'stars ' + (n ? '' : 'off')}>{'★'.repeat(v)}{'☆'.repeat(Math.max(0, 3 - v))}</span>
}

export function Pill({ p, style }: { p: PillT; style?: CSSProperties }) {
  return (
    <span className={'pill ' + p.k} style={style}>
      {p.icon && <Icon n={p.icon} />}
      {p.t}
    </span>
  )
}

// Renders trusted server HTML (brief points, verdicts). Server escapes user content.
export function Html({ html, as = 'span', className, style }: { html: string; as?: 'span' | 'div' | 'p'; className?: string; style?: CSSProperties }) {
  const Tag = as
  return <Tag className={className} style={style} dangerouslySetInnerHTML={{ __html: html }} />
}

export function Card({ id, className = '', style, children }: { id?: string; className?: string; style?: CSSProperties; children: ReactNode }) {
  return <div id={id} className={'card ' + className} style={style}>{children}</div>
}

export function Loading({ error }: { error?: string | null }) {
  return (
    <div className="card" style={{ color: error ? 'var(--bad)' : 'var(--text-3)', fontSize: 13 }}>
      {error ? 'Gagal memuat: ' + error : 'Memuat…'}
    </div>
  )
}
