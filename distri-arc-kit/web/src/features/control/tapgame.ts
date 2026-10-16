// "Tap tap Enter" while the AI reads (pure, tested): a dot orbits a ring; press Enter or Space, or tap, when it is
// inside the target arc. A hit scores, speeds the dot up and moves the target; a miss ends the streak.

export interface Game {
  angle: number // the dot, degrees 0–360 (0 at the top, clockwise)
  speed: number // degrees per second
  target: number // the target arc's centre, degrees
  width: number // the target arc, degrees
  score: number // hits in a row
  best: number
  last: 'hit' | 'miss' | null
}

export const START_SPEED = 120
const MAX_SPEED = 420
const MIN_WIDTH = 22

export function newGame(best = 0): Game {
  return { angle: 0, speed: START_SPEED, target: 180, width: 50, score: 0, best, last: null }
}

/** The dot after dt milliseconds. */
export function advance(g: Game, dt: number): Game {
  return { ...g, angle: (g.angle + (g.speed * dt) / 1000) % 360 }
}

/** The shortest distance between two angles, degrees (0–180). */
export function gap(a: number, b: number): number {
  const d = Math.abs(((a - b) % 360) + 360) % 360
  return d > 180 ? 360 - d : d
}

/** A press: inside the arc scores, speeds up and moves the target (rand: 0–1, at least 90° away); outside resets. */
export function press(g: Game, rand: number): Game {
  if (gap(g.angle, g.target) <= g.width / 2) {
    const score = g.score + 1
    return {
      ...g,
      score,
      best: Math.max(g.best, score),
      speed: Math.min(MAX_SPEED, g.speed + 18),
      width: Math.max(MIN_WIDTH, g.width - 2),
      target: (g.angle + 90 + rand * 180) % 360,
      last: 'hit',
    }
  }
  return { ...newGame(g.best), angle: g.angle, target: g.target, last: 'miss' }
}

/** A point on the ring (SVG, centre cx/cy, radius r) at an angle. */
export function point(angle: number, r: number, cx = 0, cy = 0): [number, number] {
  const rad = ((angle - 90) * Math.PI) / 180
  return [cx + r * Math.cos(rad), cy + r * Math.sin(rad)]
}

/** The SVG path of the target arc. */
export function arcPath(target: number, width: number, r: number, cx = 0, cy = 0): string {
  const [x1, y1] = point(target - width / 2, r, cx, cy)
  const [x2, y2] = point(target + width / 2, r, cx, cy)
  return `M ${x1.toFixed(2)} ${y1.toFixed(2)} A ${r} ${r} 0 0 1 ${x2.toFixed(2)} ${y2.toFixed(2)}`
}
