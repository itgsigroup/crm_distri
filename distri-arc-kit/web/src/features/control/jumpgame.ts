// "Lompat!" while the AI reads (pure, tested): a runner in the spirit of the classic platformers — the courier runs,
// Space (or Enter, or a tap) jumps over the pipes, coins in the air score extra. The world speeds up; a pipe ends the
// run, the next press starts again. Units: the 320×120 SVG of the game.

export const W = 320
export const GROUND = 100 // the ground's top
export const PX = 48 // the runner's x (left)
export const PW = 16
export const PH = 20
const GRAVITY = 900 // px/s²
const JUMP = 330 // px/s up
const START_SPEED = 120 // px/s
const MAX_SPEED = 300

export interface Pipe { x: number; w: number; h: number; id: number }
export interface Coin { x: number; y: number; id: number; got?: boolean }

export interface Run {
  y: number // the runner's feet, px above the ground (0 = on it)
  vy: number // px/s, up positive
  jumps: number // jumps since leaving the ground (two allowed: a double jump)
  speed: number
  pipes: Pipe[]
  coins: Coin[]
  dist: number // px run
  coinsGot: number
  best: number
  over: boolean
  started: boolean
  next: number // px until the next pipe
  seq: number
}

export function newRun(best = 0): Run {
  return { y: 0, vy: 0, jumps: 0, speed: START_SPEED, pipes: [], coins: [], dist: 0, coinsGot: 0, best, over: false, started: false, next: 220, seq: 0 }
}

/** The score: metres run plus 5 per coin. */
export const score = (r: Run) => Math.floor(r.dist / 20) + r.coinsGot * 5

/** Space / Enter / tap: starts, jumps (twice in the air at most), or after a crash starts a new run. */
export function jump(r: Run): Run {
  if (r.over) return { ...newRun(Math.max(r.best, score(r))), started: true, vy: JUMP, jumps: 1 }
  if (!r.started) return { ...r, started: true, vy: JUMP, jumps: 1 }
  if (r.jumps >= 2) return r
  return { ...r, vy: r.jumps === 0 ? JUMP : JUMP * 0.85, jumps: r.jumps + 1 }
}

/** The world after dt ms (rand 0–1 places the next pipe and coin). */
export function step(r: Run, dt: number, rand: number): Run {
  if (!r.started || r.over) return r
  const s = dt / 1000
  let vy = r.vy - GRAVITY * s
  let y = r.y + vy * s
  let jumps = r.jumps
  if (y <= 0) {
    y = 0
    vy = 0
    jumps = 0
  }
  const dx = r.speed * s
  let seq = r.seq
  const pipes = r.pipes.map((p) => ({ ...p, x: p.x - dx })).filter((p) => p.x + p.w > -4)
  let coins = r.coins.map((c) => ({ ...c, x: c.x - dx })).filter((c) => c.x > -10)
  let next = r.next - dx
  if (next <= 0) {
    const h = 18 + Math.round(rand * 22) // 18–40 px
    pipes.push({ x: W + 4, w: 22, h, id: ++seq })
    if (rand > 0.35) coins.push({ x: W + 4 + 11, y: h + 34 + Math.round(rand * 14), id: ++seq }) // above the pipe
    next = 150 + rand * 140 + r.speed * 0.35
  }
  // a coin: the runner's box touches it
  let got = 0
  coins = coins.map((c) => {
    if (c.got || c.x < PX - 6 || c.x > PX + PW + 6 || c.y < y - 6 || c.y > y + PH + 6) return c
    got++
    return { ...c, got: true }
  })
  // a pipe: the runner's box (a little forgiving) inside it
  const over = pipes.some((p) => p.x < PX + PW - 3 && p.x + p.w > PX + 3 && y < p.h - 2)
  const coinsGot = r.coinsGot + got
  const dist = r.dist + dx
  const run = { ...r, y, vy, jumps, pipes, coins: coins.filter((c) => !c.got || c.x > PX - 30), dist, coinsGot, next, seq, over, speed: Math.min(MAX_SPEED, START_SPEED + dist / 40) }
  return over ? { ...run, best: Math.max(r.best, score(run)) } : run
}
