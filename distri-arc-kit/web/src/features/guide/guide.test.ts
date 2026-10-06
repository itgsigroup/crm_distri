import { expect, test } from 'vitest'
import panduan from './panduan.html?raw'
import mock from '../../../../reference/distri-arc-orbit-v2-mockup.html?raw'

// The guide is the mockup's "Konsep orbit" screen verbatim: copy it again when the mockup changes.
test('panduan.html equals the mockup screen-konsep content', () => {
  const start = mock.indexOf('<section class="screen" id="screen-konsep" hidden>')
  const inner = mock.slice(mock.indexOf('\n', start) + 1, mock.indexOf('\n    </section>', start) + 1)
  expect(start).toBeGreaterThan(0)
  expect(panduan).toBe(inner)
})
