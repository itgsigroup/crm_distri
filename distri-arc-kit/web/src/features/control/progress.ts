// What the AI is analysing right now (GET /cycles/{id}/progress, Sam 17 Oct): pure helpers, tested.

export type AgentState = 'waiting' | 'reading' | 'done' | 'failed'
export interface ProgressAgent { agent: string; state: AgentState; covers?: string; done_at?: string | null; summary?: string }
export interface ProgressStep { at: string; tool: string; agent?: string; text: string; client?: string }
export interface ProgressStage { stage: string; status: string; text?: string; started_at: string | null; finished_at: string | null }
export interface CycleProgress { cycle_id: string; number: number | null; status: string; stage: string | null; via: string | null; stages: ProgressStage[]; agents: ProgressAgent[]; steps: ProgressStep[] }

export const STATE_LABEL: Record<AgentState, string> = { waiting: 'antri', reading: 'sedang dianalisa', done: 'selesai', failed: 'gagal' }

/** Agents done (or failed) of all, the bar's percent, and the one line of what the AI does now. */
export function summarize(p: Pick<CycleProgress, 'agents' | 'steps'>): { done: number; total: number; pct: number; now: string } {
  const total = p.agents.length
  const done = p.agents.filter((a) => a.state === 'done' || a.state === 'failed').length
  const reading = p.agents.filter((a) => a.state === 'reading')
  let now = ''
  if (reading.length > 0) now = 'AI sedang menganalisa ' + reading.map((a) => a.agent + (a.covers ? ` (${a.covers})` : '')).join(', ')
  else if (p.steps.length > 0) now = 'AI ' + p.steps[0].text
  else if (total > 0 && done < total) now = 'Menunggu AI mulai menganalisa'
  return { done, total, pct: total ? Math.round((done / total) * 100) : 0, now }
}

/** Reading first, then waiting, then done/failed — what is happening on top. */
export function ordered(agents: ProgressAgent[]): ProgressAgent[] {
  const rank: Record<AgentState, number> = { reading: 0, waiting: 1, failed: 2, done: 3 }
  return [...agents].sort((a, b) => rank[a.state] - rank[b.state] || a.agent.localeCompare(b.agent))
}
