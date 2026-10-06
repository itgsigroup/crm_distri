// Settles the Peta relasi layout off the main thread.
import { createLayout, positions, settle, type LayoutInput } from './layout'

self.onmessage = (e: MessageEvent<LayoutInput>) => {
  const l = settle(createLayout(e.data))
  ;(self as unknown as Worker).postMessage(positions(l))
}
