import type { CSSProperties } from 'react'

// Icons come from the mockup sprite (icons/sprite.svg), injected once by the Shell.
export function Icon({ name, className = 'i', style }: { name: string; className?: string; style?: CSSProperties }) {
  return (
    <svg className={className} style={style} aria-hidden="true">
      <use href={`#i-${name}`} />
    </svg>
  )
}
