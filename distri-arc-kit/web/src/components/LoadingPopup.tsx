import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { loadingText, useDelayedFlag } from './loading'

// A popup while a page loads data it does not have yet (first load of a query), with a 3D orbit: a planet, two
// tilted rings and their satellites. Background refreshes (SSE, polling) of data already shown never open it, and
// it does not block clicks.

const firstLoad = { predicate: (q: { state: { data: unknown; fetchStatus: string } }) => q.state.data === undefined && q.state.fetchStatus === 'fetching' }

export function LoadingPopup() {
  const qc = useQueryClient()
  const n = useIsFetching(firstLoad)
  const shown = useDelayedFlag(n > 0)
  if (!shown) return null
  const keys = qc.getQueryCache().findAll(firstLoad).map((q) => q.queryKey as unknown[])
  return (
    <div className="ld-wrap" role="status" aria-live="polite">
      <div className="ld-card">
        <div className="ld-scene" aria-hidden="true">
          <div className="ld-glow" />
          <div className="ld-3d">
            <div className="ld-orbit ld-o1"><i /></div>
            <div className="ld-orbit ld-o2"><i /></div>
            <div className="ld-planet" />
          </div>
        </div>
        <div className="ld-text">
          <b>{loadingText(keys)}</b>
          <span>GSI Orbit sedang mengambil data terbaru</span>
        </div>
        <div className="ld-bar"><i /></div>
      </div>
    </div>
  )
}
