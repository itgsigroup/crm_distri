import { useState } from 'react'

/** Shows a long list a page at a time: the first `step` rows, then "Tampilkan N lagi" — people scan the top of a
 * list; rendering thousands of rows at once only makes the screen slow. */
export function useMore<T>(items: T[], step = 20): [T[], React.ReactNode] {
  const [n, setN] = useState(step)
  const shown = items.slice(0, n)
  const rest = items.length - shown.length
  const button = rest > 0 ? (
    <button className="btn quiet more-btn" onClick={() => setN(n + step * 2)}>
      Tampilkan {Math.min(rest, step * 2)} lagi <span>· {items.length} total</span>
    </button>
  ) : n > step && items.length > step ? (
    <button className="btn quiet more-btn" onClick={() => setN(step)}>Ringkas</button>
  ) : null
  return [shown, button]
}
