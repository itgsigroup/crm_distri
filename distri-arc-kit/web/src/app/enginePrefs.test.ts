import { beforeEach, describe, expect, it, vi } from 'vitest'
import { chatgptModel, engine, modelOf, setChatgptModel, setEngine, setMcpModel } from './enginePrefs'

describe('engine of Analisis ulang (ADR 0059)', () => {
  beforeEach(() => {
    // a plain store: Node's own localStorage (no file) has no methods and hides jsdom's
    const m = new Map<string, string>()
    vi.stubGlobal('localStorage', { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k) })
  })
  it('is MCP Claude or ChatGPT; the removed Sistem reads as MCP Claude', () => {
    expect(engine()).toBe('mcp')
    localStorage.setItem('orch-engine', 'system')
    expect(engine()).toBe('mcp')
    setEngine('chatgpt')
    expect(engine()).toBe('chatgpt')
  })
  it('keeps a model per engine', () => {
    expect(chatgptModel()).toBe('gpt-5-mini')
    setChatgptModel('gpt-4.1')
    setMcpModel('claude-sonnet-5-5')
    expect(modelOf('chatgpt')).toBe('gpt-4.1')
    expect(modelOf('mcp')).toBe('claude-sonnet-5-5')
    setChatgptModel('claude-opus-5-5') // not ChatGPT's
    expect(chatgptModel()).toBe('gpt-5-mini')
  })
})
