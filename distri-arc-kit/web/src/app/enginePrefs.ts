// The model and effort an analysis asks for (16 Oct), kept per browser: for Claude through MCP the model to pick in
// claude.ai, and the effort (low · medium · high) for both engines. Sent with every "Analisis ulang".

export type Effort = 'low' | 'medium' | 'high'

export const MCP_MODELS: { id: string; label: string; short: string }[] = [
  { id: 'claude-opus-5-5', label: 'Claude Opus 5.5', short: 'Opus 5.5' },
  { id: 'claude-sonnet-5-5', label: 'Claude Sonnet 5.5', short: 'Sonnet 5.5' },
  { id: 'claude-haiku-5-5', label: 'Claude Haiku 5.5', short: 'Haiku 5.5' },
  { id: 'claude-fable-5-1', label: 'Claude Fable 5.1', short: 'Fable 5.1' },
]

/** The MCP client that analyses (19 Oct): Claude (claude.ai / Desktop) or ChatGPT (chatgpt.com) — no API key. Choosing
 * one means the other is not used for that analysis. */
export type AI = 'claude' | 'chatgpt'
export const AIS: { id: AI; label: string }[] = [
  { id: 'claude', label: 'Claude' },
  { id: 'chatgpt', label: 'ChatGPT' },
]

export const GPT_MODELS: { id: string; label: string; short: string }[] = [
  { id: 'gpt-5', label: 'GPT-5', short: 'GPT-5' },
  { id: 'gpt-5-mini', label: 'GPT-5 mini', short: 'GPT-5 mini' },
  { id: 'gpt-4.1', label: 'GPT-4.1', short: 'GPT-4.1' },
]

export const EFFORTS: { id: Effort; label: string; hint: string }[] = [
  { id: 'low', label: 'Low', hint: 'cepat & hemat token' },
  { id: 'medium', label: 'Medium', hint: 'seimbang' },
  { id: 'high', label: 'High', hint: 'paling teliti, token lebih banyak' },
]

const MODEL_KEY = 'orch-mcp-model'
const GPT_KEY = 'orch-gpt-model'
const AI_KEY = 'orch-ai'
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

export function mcpAI(): AI {
  return read(AI_KEY) === 'chatgpt' ? 'chatgpt' : 'claude'
}
export const setMcpAI = (ai: AI) => write(AI_KEY, ai)

/** The models of an AI, and the one chosen for it. */
export const modelsOf = (ai: AI) => (ai === 'chatgpt' ? GPT_MODELS : MCP_MODELS)
export function modelOf(ai: AI): string {
  if (ai === 'claude') return mcpModel()
  const v = read(GPT_KEY)
  return GPT_MODELS.some((m) => m.id === v) ? (v as string) : GPT_MODELS[0].id
}
export const setModelOf = (ai: AI, id: string) => (ai === 'claude' ? setMcpModel(id) : write(GPT_KEY, id))

export function effort(): Effort {
  const v = read(EFFORT_KEY)
  return v === 'low' || v === 'high' ? v : 'medium'
}
export const setEffort = (e: Effort) => write(EFFORT_KEY, e)

/** "Opus 5.5" for a Claude model id; the id when unknown. */
export const shortModel = (id: string) => [...MCP_MODELS, ...GPT_MODELS].find((m) => m.id === id)?.short ?? id

/** "Claude" / "ChatGPT" */
export const aiLabel = (ai: AI) => (ai === 'chatgpt' ? 'ChatGPT' : 'Claude')
