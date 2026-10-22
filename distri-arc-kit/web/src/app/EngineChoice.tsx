import { useState } from 'react'
import { Icon } from '../components/Icon'
import { CHATGPT_MODELS, EFFORTS, ENGINE_LABEL, MCP_MODELS, chatgptModel, effort, engine as savedEngine, mcpModel, setChatgptModel, setEffort, setEngine, setMcpModel, shortModel, type Effort, type Engine } from './enginePrefs'

/** The engine, model and effort of "Analisis ulang" (Sam 22 Oct: MCP Claude or ChatGPT): shared by the Orchestrator
 * dock and page — the choice is remembered in this browser and read by reanalyze. */
export function useEngine() {
  const [eng, setEng] = useState<Engine>(savedEngine)
  const [claude, setClaude] = useState(mcpModel)
  const [gpt, setGpt] = useState(chatgptModel)
  const [eff, setEff] = useState<Effort>(effort)
  return {
    engine: eng,
    model: eng === 'chatgpt' ? gpt : claude,
    effort: eff,
    choose: (e: Engine) => { setEng(e); setEngine(e) },
    pickModel: (id: string) => (eng === 'chatgpt' ? (setGpt(id), setChatgptModel(id)) : (setClaude(id), setMcpModel(id))),
    pickEffort: (e: Effort) => { setEff(e); setEffort(e) },
    claude,
    gpt,
  }
}
export type EngineState = ReturnType<typeof useEngine>

/** MCP Claude | ChatGPT, then the model of that engine and the effort. */
export function EngineChoice({ e, disabled, models = true }: { e: EngineState; disabled?: boolean; models?: boolean }) {
  return (
    <>
      <div className="dk-eng" role="radiogroup" aria-label="Dianalisis oleh">
        <button type="button" role="radio" aria-checked={e.engine === 'mcp'} className={e.engine === 'mcp' ? 'on' : ''} disabled={disabled} onClick={() => e.choose('mcp')}
          title="Claude membaca dan menganalisa lewat MCP (routine claude.ai) — tanpa kunci API">
          <Icon name="plug" /><span><b>{ENGINE_LABEL.mcp}</b><small>{shortModel(e.claude)} · {e.effort}</small></span>
        </button>
        <button type="button" role="radio" aria-checked={e.engine === 'chatgpt'} className={e.engine === 'chatgpt' ? 'on' : ''} disabled={disabled} onClick={() => e.choose('chatgpt')}
          title="ChatGPT API menganalisa di server — langsung jalan, tanpa membuka apa pun; biaya token tercatat di API & model AI">
          <Icon name="spark" /><span><b>{ENGINE_LABEL.chatgpt}</b><small>{shortModel(e.gpt)} · {e.effort}</small></span>
        </button>
      </div>
      {models && (
        <div className="dk-tune">
          <label className="dk-model">
            <span>Model</span>
            <select value={e.model} disabled={disabled} onChange={(ev) => e.pickModel(ev.target.value)}
              title={e.engine === 'chatgpt' ? 'Model ChatGPT yang menganalisa di server' : 'Pilih model yang sama di claude.ai / Desktop — perintahnya menyebut model ini'}>
              {(e.engine === 'chatgpt' ? CHATGPT_MODELS : MCP_MODELS).map((m) => <option key={m.id} value={m.id}>{m.label}</option>)}
            </select>
          </label>
          <div className="dk-effort" role="radiogroup" aria-label="Effort">
            <span>Effort</span>
            {EFFORTS.map((x) => (
              <button key={x.id} type="button" role="radio" aria-checked={e.effort === x.id} className={e.effort === x.id ? 'on' : ''} disabled={disabled} title={x.hint} onClick={() => e.pickEffort(x.id)}>{x.label}</button>
            ))}
          </div>
        </div>
      )}
    </>
  )
}
