// The engine, model and effort an analysis asks for (16 Oct; ChatGPT 22 Oct, ADR 0059), kept per browser: MCP Claude
// (the model to pick in claude.ai, or the routine) or ChatGPT (ChatGPT API on the server), and the effort (low ·
// medium · high). Sent with every "Analisis ulang" from the dock, the Orchestrator page and the Dashboard.

export type Effort = 'low' | 'medium' | 'high'

export const MCP_MODELS: { id: string; label: string; short: string }[] = [
  { id: 'claude-opus-5-5', label: 'Claude Opus 5.5', short: 'Opus 5.5' },
  { id: 'claude-sonnet-5-5', label: 'Claude Sonnet 5.5', short: 'Sonnet 5.5' },
  { id: 'claude-haiku-5-5', label: 'Claude Haiku 5.5', short: 'Haiku 5.5' },
  { id: 'claude-fable-5-1', label: 'Claude Fable 5.1', short: 'Fable 5.1' },
]

export const CHATGPT_MODELS: { id: string; label: string; short: string }[] = [
  { id: 'gpt-5-mini', label: 'GPT-5 mini', short: 'GPT-5 mini' },
  { id: 'gpt-5', label: 'GPT-5', short: 'GPT-5' },
  { id: 'gpt-4.1', label: 'GPT-4.1', short: 'GPT-4.1' },
  { id: 'gpt-4.1-mini', label: 'GPT-4.1 mini', short: 'GPT-4.1 mini' },
  { id: 'o4-mini', label: 'o4-mini', short: 'o4-mini' },
]

/** The engine of "Analisis ulang": Claude through MCP, or ChatGPT (API on the server). */
export type Engine = 'mcp' | 'chatgpt'
export const ENGINE_LABEL: Record<Engine, string> = { mcp: 'MCP Claude', chatgpt: 'ChatGPT' }

export const EFFORTS: { id: Effort; label: string; hint: string }[] = [
  { id: 'low', label: 'Low', hint: 'cepat & hemat token' },
  { id: 'medium', label: 'Medium', hint: 'seimbang' },
  { id: 'high', label: 'High', hint: 'paling teliti, token lebih banyak' },
]

const MODEL_KEY = 'orch-mcp-model'
const GPT_KEY = 'orch-chatgpt-model'
const ENGINE_KEY = 'orch-engine'
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

export function chatgptModel(): string {
  const v = read(GPT_KEY)
  return CHATGPT_MODELS.some((m) => m.id === v) ? (v as string) : CHATGPT_MODELS[0].id
}
export const setChatgptModel = (id: string) => write(GPT_KEY, id)

/** The engine chosen last; "Sistem" (removed 22 Oct) and older choices read as MCP Claude. */
export function engine(): Engine {
  return read(ENGINE_KEY) === 'chatgpt' ? 'chatgpt' : 'mcp'
}
export const setEngine = (e: Engine) => write(ENGINE_KEY, e)

/** The model of an engine. */
export const modelOf = (e: Engine) => (e === 'chatgpt' ? chatgptModel() : mcpModel())

export function effort(): Effort {
  const v = read(EFFORT_KEY)
  return v === 'low' || v === 'high' ? v : 'medium'
}
export const setEffort = (e: Effort) => write(EFFORT_KEY, e)

/** "Opus 5.5" for a Claude model id; the id when unknown. */
export const shortModel = (id: string) => [...MCP_MODELS, ...CHATGPT_MODELS].find((m) => m.id === id)?.short ?? id
