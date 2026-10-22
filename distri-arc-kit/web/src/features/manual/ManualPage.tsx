import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { Icon } from '../../components/Icon'
import { Markdown } from '../../components/Markdown'
import { ROUTES } from '../../app/Shell'
import { TITLES } from '../../lib/i18n/id'
import { CHAPTERS, searchChapters } from './chapters'

// Buku manual (Sam 22 Oct): the user book of GSI Orbit — how the system works, a tutorial per menu, the daily routine
// per role, FAQ. The table of contents with a search on the left, a chapter on the right (?bab= keeps it linkable),
// "Buka halaman ini" to the menu it explains, and Cetak (the whole book) for a PDF or paper copy.
export function ManualPage() {
  const [params, setParams] = useSearchParams()
  const nav = useNavigate()
  const [q, setQ] = useState('')
  const [printAll, setPrintAll] = useState(false)
  const id = params.get('bab') ?? CHAPTERS[0].id
  const idx = Math.max(0, CHAPTERS.findIndex((c) => c.id === id))
  const ch = CHAPTERS[idx]
  const found = useMemo(() => searchChapters(q), [q])
  const groups = useMemo(() => [...new Set(found.map((c) => c.group))], [found])
  const body = useRef<HTMLDivElement>(null)
  const open = (next: string) => {
    setParams({ bab: next })
    body.current?.scrollIntoView({ block: 'start', behavior: 'smooth' })
  }
  // Cetak: render every chapter, print, then back to the one chapter
  useEffect(() => {
    if (!printAll) return
    const t = setTimeout(() => { window.print(); setPrintAll(false) }, 50)
    return () => clearTimeout(t)
  }, [printAll])
  return (
    <div className="man">
      <aside className="card man-toc" aria-label="Daftar isi buku manual">
        <div className="man-search"><Icon name="search" /><input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Cari di manual…" aria-label="Cari di manual" /></div>
        <button className="btn ghost man-print" onClick={() => setPrintAll(true)}><Icon name="doc" />Cetak / simpan PDF</button>
        <select className="man-select" value={ch.id} onChange={(e) => open(e.target.value)} aria-label="Pilih bab">
          {found.map((c, i) => <option key={c.id} value={c.id}>{i + 1}. {c.title}</option>)}
        </select>
        {found.length === 0 && <p className="man-none">Tidak ada bab yang memuat "{q}".</p>}
        {groups.map((g) => (
          <div key={g} className="man-grp">
            <span>{g}</span>
            {found.filter((c) => c.group === g).map((c) => (
              <button key={c.id} className={c.id === ch.id ? 'on' : ''} onClick={() => open(c.id)} aria-current={c.id === ch.id ? 'page' : undefined}>
                <Icon name={c.icon} />{c.title}
              </button>
            ))}
          </div>
        ))}
      </aside>
      <article className="card man-body" ref={body}>
        {printAll ? (
          <>
            <h1 className="man-cover">Buku Manual GSI Orbit</h1>
            {CHAPTERS.map((c) => <section key={c.id} className="man-md man-page"><Markdown text={c.md} /></section>)}
          </>
        ) : (
          <>
            <div className="man-head">
              <span className="man-crumb">{ch.group} · Bab {idx + 1} dari {CHAPTERS.length}</span>
              {ch.screen && <button className="btn primary" onClick={() => nav(ROUTES[ch.screen!])}><Icon name="arrow" />Buka halaman {TITLES[ch.screen][0]}</button>}
            </div>
            <div className="man-md"><Markdown text={ch.md} /></div>
            <div className="man-nav">
              {idx > 0 ? <button className="btn quiet" onClick={() => open(CHAPTERS[idx - 1].id)}><Icon name="arrow" style={{ transform: 'rotate(180deg)' }} />{CHAPTERS[idx - 1].title}</button> : <span />}
              {idx < CHAPTERS.length - 1 && <button className="btn ghost" onClick={() => open(CHAPTERS[idx + 1].id)}>{CHAPTERS[idx + 1].title}<Icon name="arrow" /></button>}
            </div>
          </>
        )}
      </article>
    </div>
  )
}
