// Hari ini: greeting, status strip, brief, decision queue, commitments, signals,
// tomorrow's agenda, forecast, business pulse, renewal & expansion, agents feed.
// Ported from <section id="screen-today"> in reference/arc-crm-mockup.html.
// Every value shown here comes from GET /api/today.
import { useState } from 'react'
import { api, useApi } from '../api/client'
import type { Action, DecisionRequest, DecisionResponse, Pill as PillT, Tone, Today } from '../api/types'
import { Html, Icon, Loading, Pill } from '../components/ui'
import { fmtRp1 } from '../lib/format'
import { useUI } from '../state/ui'

const TOUR_KEY = 'arc-tour'

// CSS color for a tone; "neutral"/"muted" have no token of their own in the mockup.
const toneVar = (t: Tone | 'muted' | undefined): string =>
  !t || t === 'neutral' || t === 'muted' ? 'var(--text-3)' : `var(--${t})`
const toneSoft = (t: Tone): string => (t === 'neutral' ? 'var(--surface-3)' : `var(--${t}-soft)`)

// Server HTML wrapped so the global delegated handler in App.tsx follows data-go buttons.
function GoHtml({ html }: { html: string }) {
  return (
    <span className="html-go">
      <Html html={html} />
    </span>
  )
}

function readTourDismissed(): boolean {
  try {
    return localStorage.getItem(TOUR_KEY) === '1'
  } catch {
    return false
  }
}

// Pill with the extra "st" class used by the commitment ledger rows.
function StPill({ p }: { p: PillT }) {
  return (
    <span className={'st pill ' + p.k}>
      {p.icon && <Icon n={p.icon} />}
      {p.t}
    </span>
  )
}

export function TodayScreen() {
  const { go } = useUI()
  const { data, error } = useApi<Today>('/api/today')
  const [tourHidden, setTourHidden] = useState(readTourDismissed)
  const [agentsOpen, setAgentsOpen] = useState(false)

  if (!data) {
    return (
      <section className="screen" id="screen-today">
        <Loading error={error} />
      </section>
    )
  }

  const onStrip = (s: Today['strip'][number]) => {
    if (s.scroll) document.getElementById(s.scroll)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    else if (s.go) go(s.go)
  }
  const closeTour = () => {
    setTourHidden(true)
    try {
      localStorage.setItem(TOUR_KEY, '1')
    } catch {
      /* storage unavailable: tour simply reappears next visit */
    }
  }
  // The contract has no account id on renewal rows; use the first account link in their HTML.
  const renewalTarget = (() => {
    for (const r of data.renewal.rows) {
      const m = /data-go="(rel:[^"]+)"/.exec(r.html)
      if (m) return m[1]
    }
    return 'rel'
  })()
  const { brief, forecast } = data

  return (
    <section className="screen" id="screen-today">
      <div className="greet">
        <h2>{data.greeting.title}</h2>
        <p>{data.greeting.sub}</p>
      </div>

      {data.strip.length > 0 && (
        <div className="status-strip">
          {data.strip.map((s, i) => (
            <button key={i} onClick={() => onStrip(s)}>
              <span className="n" style={{ background: toneVar(s.tone) }}>{s.n}</span>
              {s.label}
              {s.muted && <span className="m">· {s.muted.replace(/^·\s*/, '')}</span>}
            </button>
          ))}
        </div>
      )}

      {!tourHidden && (
        <div className="tour" id="tour">
          <span className="ai">Cara pakai ARC</span>
          <ol>
            <li><i>1</i><b>Baca brief</b> — {brief ? brief.points.length + ' kalimat' : 'beberapa kalimat'}, sudah diringkas</li>
            <li><i>2</i><b>Putuskan</b> — setujui, edit, atau tolak saran</li>
            <li><i>3</i><b>Tanya</b> apa saja lewat kotak di atas (⌘K)</li>
          </ol>
          <button className="btn quiet" onClick={closeTour}>Tutup</button>
        </div>
      )}

      <div className="today">
        <div className="stack">
          {brief && (
            <div className="card brief">
              <div className="card-h">
                <span className="ai">{brief.written_label}</span>
                <span className="meta">{brief.meta}</span>
              </div>
              <ol>
                {brief.points.map((p, i) => (
                  <li key={i}>
                    <span className={'k ' + p.k}><Icon n={p.icon} /></span>
                    <div><GoHtml html={p.html} /></div>
                  </li>
                ))}
              </ol>
              <div className="foot">
                {brief.sources.map((s, i) => (
                  <span className="prov" key={i}><Icon n={s.icon} />{s.label}</span>
                ))}
                <span className="prov" style={{ marginLeft: 'auto' }}>confidence {brief.confidence.toFixed(2)}</span>
              </div>
            </div>
          )}

          <Queue queue={data.queue} />

          <div className="grid-2">
            <div className="card" id="commit-card">
              <div className="card-h"><h2>Komitmen hari ini</h2><span className="meta">Ledger dua arah</span></div>
              <ul className="row-list">
                {data.commitments.map((c, i) => (
                  <li key={i}>
                    <span className="who">{c.who}</span>
                    <div><div className="t">{c.t}</div><div className="s">{c.s}</div></div>
                    <StPill p={c.pill} />
                  </li>
                ))}
              </ul>
            </div>
            <div className="card" id="signal-card">
              <div className="card-h"><h2>Sinyal baru</h2><span className="meta">24 jam terakhir</span></div>
              <div className="sig">
                {data.signals.map(s => (
                  <button key={s.id} onClick={() => go(s.go)}>
                    <span className="si" style={{ background: toneSoft(s.k), color: toneVar(s.k) }}><Icon n={s.icon} /></span>
                    <div><b>{s.title}</b><span>{s.detail}</span></div>
                    <Icon n="i-arrow" cls="i arrow" />
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>

        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Besok</h2><span className="meta">{data.tomorrow.label}</span></div>
            <ul className="agenda">
              {data.tomorrow.items.map((it, i) => (
                <li key={i}>
                  <div className="tm">{it.time}<small>{it.duration}</small></div>
                  <div>
                    <b>{it.title}</b>
                    <span>{it.detail}</span>
                    {it.pills.length > 0 && (
                      <div className="prep">
                        {it.pills.map((p, j) => (
                          <span key={j}>{j > 0 && ' '}<Pill p={p} /></span>
                        ))}
                      </div>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          </div>

          <div className="card">
            <div className="card-h"><h2>{forecast.title}</h2><span className="meta">{forecast.meta}</span></div>
            <div className="fc">
              {forecast.rows.map((r, i) => (
                <div className="row" key={i}>
                  <span className="lbl">{r.label}</span>
                  <div className="bar">
                    <i className={r.variant || undefined} style={{ width: r.width + '%' }} />
                    <span className="tgt" style={{ left: forecast.target_pos + '%' }} />
                  </div>
                  <span className="v num">{fmtRp1(r.value)}</span>
                </div>
              ))}
              {forecast.note && <p className="note">{forecast.note}</p>}
            </div>
          </div>

          <div className="card">
            <div className="card-h"><h2>Denyut bisnis</h2><span className="meta">{data.pulse.meta}</span></div>
            <ul className="pulse">
              {data.pulse.rows.map((r, i) => (
                <li key={i}>
                  <span className="lbl">{r.label}</span>
                  <em className="num">{r.value}</em>
                  <span className={'dl ' + r.tone}>{r.delta}</span>
                </li>
              ))}
            </ul>
            <button className="btn ghost" style={{ width: '100%', justifyContent: 'center', marginTop: 12 }} onClick={() => go('cash')}>
              <Icon n="i-cash" />Buka Cash engine
            </button>
          </div>

          <div className="card">
            <div className="card-h"><h2>Renewal &amp; ekspansi</h2><span className="ai" style={{ marginLeft: 6 }}>Research agent</span></div>
            <ul className="row-list">
              {data.renewal.rows.map((r, i) => (
                <li key={i}>
                  <span className="who" style={{ width: 'auto', color: toneVar(r.tone) }}>
                    <Icon n={r.icon} style={{ width: 16, height: 16 }} />
                  </span>
                  <div>
                    <div className="t"><GoHtml html={r.html} /></div>
                    <div className="s">{r.sub}</div>
                  </div>
                </li>
              ))}
            </ul>
            <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
              <button className="btn ghost" onClick={() => go(renewalTarget)}>Lihat peluang di akun</button>
              <button className="btn ghost" onClick={() => go('pipe')}>Tender radar</button>
            </div>
          </div>

          <div className={'agents-line' + (agentsOpen ? ' open' : '')} onClick={() => setAgentsOpen(o => !o)}>
            <span className="dot-live" />
            <GoHtml html={data.agents_line.summary_html} />
            <Icon n="i-chev" cls="i chev" />
          </div>
          <div className="agents-body">
            <ul className="feed">
              {data.agents_line.feed.map((f, i) => (
                <li key={i}><span className="ag">{f.agent}</span><div><GoHtml html={f.html} /></div></li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}

// ---------- Keputusan (approval queue) ----------

function Queue({ queue }: { queue: Today['queue'] }) {
  const { toast, refresh, openSheet, ask } = useUI()
  const [filter, setFilter] = useState('all')
  const [toggled, setToggled] = useState<Record<string, boolean>>({})
  const [done, setDone] = useState<Record<string, string>>({}) // id -> result text
  const [gone, setGone] = useState<Set<string>>(() => new Set())
  const [hidden, setHidden] = useState<Set<string>>(() => new Set())
  const [busy, setBusy] = useState<string | null>(null)

  const decide = async (a: Action, req: DecisionRequest) => {
    if (busy) return
    setBusy(a.id)
    try {
      const r = await api.post<DecisionResponse>('/api/actions/' + encodeURIComponent(a.id) + '/decision', req)
      toast(r.toast)
      if (req.decision === 'snooze') {
        setGone(s => new Set(s).add(a.id))
        setTimeout(() => setHidden(s => new Set(s).add(a.id)), 300)
      } else {
        setDone(m => ({ ...m, [a.id]: r.action.result_text || a.result_text }))
      }
      refresh()
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  const firstId = queue.items[0]?.id
  const visible = queue.items.filter(a => !hidden.has(a.id) && (filter === 'all' || a.kind === filter))

  return (
    <div className="card" id="queue-card">
      <div className="card-h"><h2>Keputusan</h2><span className="meta" id="queue-meta">{queue.meta}</span></div>
      {queue.filters.length > 0 && (
        <div className="qfilters">
          {queue.filters.map(f => (
            <button key={f.key} className={filter === f.key ? 'is-active' : ''} onClick={() => setFilter(f.key)}>{f.label}</button>
          ))}
        </div>
      )}
      <div className="queue">
        {visible.map(a => {
          const isDone = a.status !== 'proposed' || a.id in done
          const open = toggled[a.id] ?? a.id === firstId
          const hasOptions = a.options.length > 0
          const options = [...a.options].sort((x, y) => Number(!!y.primary) - Number(!!x.primary))
          // The quick button is the agent's recommended option (button_label), not necessarily the primary one.
          const quickOpt = options.find(o => o.label === a.button_label) ?? options[0]
          const approve = () => decide(a, { decision: 'approve' })
          const pick = (key: string) => decide(a, { decision: 'option', option: key })
          const dis = busy === a.id
          const cls = 'q' + (open ? ' open' : '') + (isDone ? ' done' : '') + (gone.has(a.id) ? ' gone' : '')
          return (
            <div className={cls} key={a.id} id={'q-' + a.id} data-kind={a.kind}>
              <div className="qi" style={{ color: toneVar(a.tags[0]?.k ?? 'accent') }}><Icon n={a.icon} /></div>
              <div>
                <div
                  className="q-head"
                  onClick={e => {
                    if ((e.target as HTMLElement).closest('button')) return
                    setToggled(t => ({ ...t, [a.id]: !open }))
                  }}
                >
                  <div className="qt">
                    <b>{a.title}</b>
                    {a.tags.map((p, i) => <Pill key={i} p={p} />)}
                  </div>
                  {!isDone && (
                    <div className="q-quick">
                      {hasOptions ? (
                        <button className={'btn ' + (quickOpt.primary ? 'primary' : 'ghost')} disabled={dis} onClick={() => pick(quickOpt.key)}>{quickOpt.label}</button>
                      ) : (
                        <button className="btn primary" disabled={dis} onClick={approve}><Icon n={a.icon} />{a.button_label}</button>
                      )}
                    </div>
                  )}
                  <Icon n="i-chev" cls="i chev" />
                </div>
                <div className="q-body">
                  <div className="qd">{a.summary}</div>
                  {a.preview && (
                    <div className="qp">
                      {a.preview_from && <div className="from">{a.preview_from}</div>}
                      {a.preview}
                    </div>
                  )}
                  {a.impact.length > 0 && (
                    <div className="impact">
                      {a.impact.map((im, i) => (
                        <div key={i}>
                          {im.label}
                          <b className="num" style={im.tone ? { color: toneVar(im.tone) } : undefined}>{im.value}</b>
                        </div>
                      ))}
                    </div>
                  )}
                  {a.context_note && (
                    <div className="qw"><span className="ai" /><span>{a.context_note}</span></div>
                  )}
                  <div className="qa">
                    {hasOptions ? (
                      <>
                        {options.map((o, i) => (
                          <button key={o.key} className={'btn ' + (i === 0 ? 'primary' : 'ghost')} disabled={dis} onClick={() => pick(o.key)}>{o.label}</button>
                        ))}
                        <button className="btn quiet" onClick={() => ask('Diskusikan: ' + a.title)}>Diskusikan</button>
                      </>
                    ) : (
                      <>
                        <button className="btn primary" disabled={dis} onClick={approve}><Icon n={a.icon} />{a.button_label}</button>
                        <button className="btn ghost" onClick={() => openSheet(a)}><Icon n="i-edit" />Edit</button>
                        <button className="btn quiet" disabled={dis} onClick={() => decide(a, { decision: 'snooze' })}>Lewati</button>
                      </>
                    )}
                  </div>
                  <div className="qres"><Icon n="i-check" /><span>{done[a.id] ?? a.result_text}</span></div>
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
