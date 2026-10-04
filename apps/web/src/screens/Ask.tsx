// Ask ARC: conversation thread (history from the API + questions asked in this
// session), evidence cards, scenarios, recommendations and the composer.
// Ported from <section id="screen-ask"> in reference/arc-crm-mockup.html.
import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { api, useApi } from '../api/client'
import type { AskAnswer, AskRec } from '../api/types'
import { Html, Icon } from '../components/ui'
import { fmtRp } from '../lib/format'
import { useUI } from '../state/ui'

interface Pending {
  key: number
  question: string
  answer: AskAnswer | null // null while ARC is thinking
  error?: string
}

// Server HTML wrapped so the global delegated handler in App.tsx follows data-go buttons.
function GoHtml({ html }: { html: string }) {
  return (
    <span className="html-go">
      <Html html={html} />
    </span>
  )
}

const scrollToBottom = () => {
  requestAnimationFrame(() => window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' }))
}

export function AskScreen() {
  const { pendingAsk, consumeAsk, toast } = useUI()
  const { data: history } = useApi<AskAnswer[]>('/api/ask/history')
  const { data: suggestions } = useApi<string[]>('/api/ask/suggestions?screen=ask')
  const [extra, setExtra] = useState<Pending[]>([])
  const [input, setInput] = useState('')
  const seq = useRef(0)
  const consumed = useRef<string | null>(null)

  const submit = useCallback((question: string) => {
    const q = question.trim()
    if (!q) return
    const key = ++seq.current
    setExtra(list => [...list, { key, question: q, answer: null }])
    scrollToBottom()
    api
      .post<AskAnswer>('/api/ask', { question: q, screen: 'ask' })
      .then(answer => setExtra(list => list.map(p => (p.key === key ? { ...p, answer } : p))))
      .catch(e => {
        const msg = e instanceof Error ? e.message : String(e)
        toast(msg)
        setExtra(list => list.map(p => (p.key === key ? { ...p, error: msg } : p)))
      })
      .finally(scrollToBottom)
  }, [toast])

  // Questions handed over from the top search box (ask()). Guarded so StrictMode's
  // double effect run cannot submit the same hand-over twice.
  useEffect(() => {
    if (!pendingAsk) {
      consumed.current = null
      return
    }
    if (consumed.current === pendingAsk) return
    consumed.current = pendingAsk
    const q = consumeAsk()
    if (q) submit(q)
  }, [pendingAsk, consumeAsk, submit])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const q = input.trim()
    if (!q) return
    setInput('')
    submit(q)
  }

  // A refresh can bring answers asked here back via history; don't show them twice.
  const historyIds = new Set((history ?? []).map(a => a.id))
  const local = extra.filter(p => !p.answer || !historyIds.has(p.answer.id))

  return (
    <section className="screen ask" id="screen-ask">
      <div className="thread" id="thread">
        {(history ?? []).map(a => (
          <Exchange key={a.id} question={a.question} answer={a} />
        ))}
        {local.map(p => (
          <Exchange key={'l' + p.key} question={p.question} answer={p.answer} error={p.error} />
        ))}
      </div>
      <div className="composer">
        {suggestions && suggestions.length > 0 && (
          <div className="chips">
            {suggestions.map(s => (
              <button type="button" key={s} className="chip" onClick={() => submit(s)}>{s}</button>
            ))}
          </div>
        )}
        <form id="ask-form" onSubmit={onSubmit}>
          <input
            id="ask-input"
            type="text"
            placeholder="Tanya apa saja tentang relasi Anda…"
            autoComplete="off"
            aria-label="Pertanyaan"
            value={input}
            onChange={e => setInput(e.target.value)}
          />
          <button type="button" className="mic" aria-label="Voice memo" onClick={() => toast('Voice memo diproses Capture agent')}>
            <Icon n="i-mic" />
          </button>
          <button type="submit" className="send" aria-label="Kirim"><Icon n="i-arrow" /></button>
        </form>
      </div>
    </section>
  )
}

function Exchange({ question, answer, error }: { question: string; answer: AskAnswer | null; error?: string }) {
  return (
    <>
      <div className="msg user"><div className="bubble">{question}</div></div>
      <div className="msg">
        {answer ? (
          <Reply a={answer} />
        ) : error ? (
          <div className="reply">
            <div className="who"><span className="ai">ARC</span>· gagal menjawab</div>
            <p className="txt" style={{ color: 'var(--bad)' }}>{error}</p>
          </div>
        ) : (
          <div className="reply">
            <div className="who"><span className="ai">ARC</span>· memeriksa graph</div>
            <div className="thinking"><i /><i /><i /></div>
          </div>
        )}
      </div>
    </>
  )
}

function Reply({ a }: { a: AskAnswer }) {
  const { go, openSheet, toast } = useUI()
  const onRec = (r: AskRec) => {
    if (r.action) openSheet(r.action)
    else if (r.toast) toast(r.toast)
  }
  const showSrc = a.sources.length > 0 || a.confidence > 0
  return (
    <div className="reply">
      <div className="who"><span className="ai">ARC</span>· {a.who_label}</div>
      {a.paragraphs.map((p, i) => (
        <p className="txt" key={i}><GoHtml html={p} /></p>
      ))}
      {a.evidence.length > 0 && (
        <div className="evs">
          {a.evidence.map(ev => (
            <button className="evc" key={ev.account_id} onClick={() => go('rel:' + ev.account_id)}>
              <div className="top">
                <span className={'dot ' + ev.band} />
                <b>{ev.name}</b>
                <span className="val num">{fmtRp(ev.value)}</span>
              </div>
              <ul>
                {ev.items.map((it, i) => (
                  <li key={i}><Icon n={it.icon} />{it.text}</li>
                ))}
              </ul>
              {ev.provs.length > 0 && (
                <div className="src">
                  {ev.provs.map((pv, i) => <span className="prov" key={i}>{pv}</span>)}
                </div>
              )}
            </button>
          ))}
        </div>
      )}
      {a.scenario.length > 0 && (
        <div className="scen">
          {a.scenario.map((r, i) => (
            <div className="row" key={i}>
              <span className="lbl">{r.label}</span>
              <div className="bar">
                <i className={r.was ? 'was' : undefined} style={{ width: r.width + '%' }} />
                {r.target_pos !== undefined && r.target_pos !== null && <span className="tgt" style={{ left: r.target_pos + '%' }} />}
              </div>
              <span className="v num">{r.value}{r.delta && <> <small>{r.delta}</small></>}</span>
            </div>
          ))}
        </div>
      )}
      {a.followup && <p className="txt"><GoHtml html={a.followup} /></p>}
      {a.recs.length > 0 && (
        <div className="recs">
          {a.recs.map((r, i) => (
            <button key={i} className={'btn ' + (r.primary ? 'primary' : 'ghost')} onClick={() => onRec(r)}>
              {r.icon && <Icon n={r.icon} />}
              {r.label}
            </button>
          ))}
        </div>
      )}
      {showSrc && (
        <div className="src" style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
          {a.sources.map((s, i) => <span className="prov" key={i}>{s}</span>)}
          {a.confidence > 0 && <span className="prov">conf {a.confidence.toFixed(2)}</span>}
        </div>
      )}
    </div>
  )
}
