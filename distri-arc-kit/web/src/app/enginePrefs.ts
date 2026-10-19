// The model and effort an analysis asks for (16 Oct), kept per browser: for Claude through MCP the model to pick in
// claude.ai, and the effort (low · medium · high) for both engines. Sent with every "Analisis ulang".

export type Effort = 'low' | 'medium' | 'high'

export const MCP_MODELS: { id: string; label: string; short: string }[] = [
  { id: 'claude-opus-5-5', label: 'Claude Opus 5.5', short: 'Opus 5.5' },
  { id: 'claude-sonnet-5-5', label: 'Claude Sonnet 5.5', short: 'Sonnet 5.5' },
  { id: 'claude-haiku-5-5', label: 'Claude Haiku 5.5', short: 'Haiku 5.5' },
  { id: 'claude-fable-5-1', label: 'Claude Fable 5.1', short: 'Fable 5.1' },
]

export const EFFORTS: { id: Effort; label: string; hint: string }[] = [
  { id: 'low', label: 'Low', hint: 'cepat & hemat token' },
  { id: 'medium', label: 'Medium', hint: 'seimbang' },
  { id: 'high', label: 'High', hint: 'paling teliti, token lebih banyak' },
]

const MODEL_KEY = 'orch-mcp-model'
const EFFORT_KEY = 'orch-effort'

function read(key: string): string | null {
  try { return localStorage.getItem(key) } catch { return null }
}
function write(key: string, v: string) {
  try { localStorage.setItem(key, v) } catch { /* private mode */ }
}

export function mcpModel(): string {
  const v = read(MODEL_KEY)
  return MCP_MODELS.some((m) => m.id === v) ? (v as string) : MCP_MODELS[0].id
}
export const setMcpModel = (id: string) => write(MODEL_KEY, id)

export function effort(): Effort {
  const v = read(EFFORT_KEY)
  return v === 'low' || v === 'high' ? v : 'medium'
}
export const setEffort = (e: Effort) => write(EFFORT_KEY, e)

/** "Opus 5.5" for a Claude model id; the id when unknown. */
export const shortModel = (id: string) => MCP_MODELS.find((m) => m.id === id)?.short ?? id
