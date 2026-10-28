import { useState } from 'react'
import { HermesChat, HermesChatConnect } from './HermesChat'

const TABS = [['chat', 'Chat'], ['setup', 'Sambungan']] as const
type Tab = (typeof TABS)[number][0]

/** Sidebar → Hermes Agent: the place to talk to Lyra, the company's Hermes Agent, like in Telegram (ADR 0067). How
 * Hermes reads GSI Orbit over MCP (tokens, YAML) lives on the MCP Claude page, not here. */
export function HermesPage() {
  const [tab, setTab] = useState<Tab>(() => (window.location.hash === '#sambungan' ? 'setup' : 'chat'))
  const go = (t: Tab) => { setTab(t); window.history.replaceState(null, '', t === 'setup' ? '#sambungan' : window.location.pathname) }
  return (
    <div className="stack">
      <div className="seg hc-tabs" role="tablist" aria-label="Lyra · Hermes Agent">
        {TABS.map(([k, label]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'is-active' : ''} onClick={() => go(k)}>{label}</button>)}
      </div>
      {tab === 'chat' ? <HermesChat onSettings={() => go('setup')} /> : <HermesChatConnect />}
    </div>
  )
}
