import type { ReactNode } from 'react'

// A small, safe Markdown renderer for analysis reports: headings, paragraphs, lists, quotes, tables, **bold**,
// _italic_ and `code`. It builds React elements (no innerHTML), so model output can never inject markup.

function inline(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = []
  const re = /(\*\*[^*]+\*\*|`[^`]+`|_[^_]+_)/g
  let last = 0
  let m: RegExpExecArray | null
  let i = 0
  while ((m = re.exec(text))) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const t = m[0]
    const k = `${key}-${i++}`
    if (t.startsWith('**')) out.push(<b key={k}>{t.slice(2, -2)}</b>)
    else if (t.startsWith('`')) out.push(<code key={k}>{t.slice(1, -1)}</code>)
    else out.push(<i key={k}>{t.slice(1, -1)}</i>)
    last = m.index + t.length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

const cells = (line: string) => line.trim().replace(/^\||\|$/g, '').split('|').map((c) => c.trim())
const isRule = (line: string) => /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/.test(line)

export function Markdown({ text }: { text: string }) {
  const lines = text.replace(/\r/g, '').split('\n')
  const blocks: ReactNode[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    const k = `b${i}`
    if (!line.trim()) { i++; continue }
    const h = /^(#{1,4})\s+(.*)$/.exec(line)
    if (h) {
      const lvl = h[1].length
      blocks.push(lvl <= 2 ? <h3 key={k}>{inline(h[2], k)}</h3> : <h4 key={k}>{inline(h[2], k)}</h4>)
      i++
      continue
    }
    if (line.trim().startsWith('|') && i + 1 < lines.length && isRule(lines[i + 1])) {
      const head = cells(line)
      const align = cells(lines[i + 1]).map((c) => (c.endsWith(':') ? 'right' : 'left') as 'right' | 'left')
      const rows: string[][] = []
      i += 2
      while (i < lines.length && lines[i].trim().startsWith('|')) rows.push(cells(lines[i++]))
      blocks.push(
        <div key={k} className="tbl-wrap"><table className="tbl">
          <thead><tr>{head.map((c, j) => <th key={j} style={{ textAlign: align[j] }}>{inline(c, `${k}h${j}`)}</th>)}</tr></thead>
          <tbody>{rows.map((r, ri) => <tr key={ri}>{r.map((c, j) => <td key={j} style={{ textAlign: align[j] }}>{inline(c, `${k}r${ri}c${j}`)}</td>)}</tr>)}</tbody>
        </table></div>,
      )
      continue
    }
    if (/^\s*([-*]|\d+\.)\s+/.test(line)) {
      const ordered = /^\s*\d+\./.test(line)
      const items: string[] = []
      while (i < lines.length && /^\s*([-*]|\d+\.)\s+/.test(lines[i])) items.push(lines[i++].replace(/^\s*([-*]|\d+\.)\s+/, ''))
      const lis = items.map((it, j) => <li key={j}>{inline(it, `${k}l${j}`)}</li>)
      blocks.push(ordered ? <ol key={k}>{lis}</ol> : <ul key={k}>{lis}</ul>)
      continue
    }
    if (line.startsWith('>')) {
      const q: string[] = []
      while (i < lines.length && lines[i].startsWith('>')) q.push(lines[i++].replace(/^>\s?/, ''))
      blocks.push(<blockquote key={k}>{inline(q.join(' '), k)}</blockquote>)
      continue
    }
    const p: string[] = []
    while (i < lines.length && lines[i].trim() && !/^(#{1,4}\s|>|\s*([-*]|\d+\.)\s+|\s*\|)/.test(lines[i])) p.push(lines[i++])
    if (!p.length) p.push(lines[i++])
    blocks.push(<p key={k}>{inline(p.join(' '), k)}</p>)
  }
  return <div className="md">{blocks}</div>
}
