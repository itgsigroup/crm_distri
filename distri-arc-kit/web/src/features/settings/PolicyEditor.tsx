import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { hhmm, shortDate } from '../../lib/format'

export interface PolicyField {
  path: string[]
  label: string
  unit?: string // "×", "%", "hari", "Rp"
  step?: number
  min?: number
  max?: number
  hint?: string
}

type Obj = Record<string, unknown>

function getIn(o: unknown, path: string[]): unknown {
  return path.reduce<unknown>((v, k) => (v && typeof v === 'object' ? (v as Obj)[k] : undefined), o)
}

function setIn(o: Obj, path: string[], v: unknown): Obj {
  const out: Obj = { ...o }
  if (path.length === 1) {
    out[path[0]] = v
    return out
  }
  out[path[0]] = setIn((o[path[0]] as Obj) ?? {}, path.slice(1), v)
  return out
}

/** Save a whole policy value (schema-validated server side, new version). */
export function useSavePolicy() {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: unknown }) => api.put<{ message: string; version: number }>(`/policies/${key}`, value),
    onSuccess: (r) => {
      toast(`${r.message} · versi ${r.version}`)
      for (const k of ['policies', 'mcp', 'agents']) qc.invalidateQueries({ queryKey: [k] })
    },
    onError: (e: Error) => toast(e.message),
  })
}

interface HistoryRow { version: number; value: Obj; updated_at: string; updated_by_name: string | null }

/** Sheet that edits some fields of one policy, with its version history. */
export function PolicyEditor({ policyKey, title, value, fields }: { policyKey: string; title: string; value: Obj; fields: PolicyField[] }) {
  const { closeSheet } = useFeedback()
  const save = useSavePolicy()
  const [draft, setDraft] = useState<Obj>(value)
  const { data: hist } = useQuery({ queryKey: ['policies', 'history', policyKey], queryFn: () => api.get<{ items: HistoryRow[] }>(`/policies/${policyKey}/history?limit=5`).then((r) => r.items) })
  const input = { width: 140, height: 34, borderRadius: 9, border: '1px solid var(--line)', padding: '0 10px', font: 'inherit', fontSize: 14, background: 'var(--surface)', color: 'var(--text)', textAlign: 'right' as const }
  return (
    <>
      <SheetHead icon="gear" title={title} sub={`Kebijakan ${policyKey} · berlaku di siklus berikutnya`} onClose={closeSheet} />
      <div className="sec">
        <ul className="rules">
          {fields.map((f) => {
            const v = getIn(draft, f.path)
            const isRp = f.unit === 'Rp'
            const shown = typeof v === 'number' ? (isRp ? v / 1e6 : v) : ''
            return (
              <li key={f.path.join('.')}>
                <div><b>{f.label}</b>{f.hint && <span>{f.hint}</span>}</div>
                <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <input
                    style={input}
                    type="number"
                    step={isRp ? 1 : (f.step ?? 1)}
                    min={f.min}
                    max={f.max}
                    value={shown}
                    aria-label={f.label}
                    onChange={(e) => {
                      const n = e.target.value === '' ? 0 : Number(e.target.value)
                      setDraft(setIn(draft, f.path, isRp ? Math.round(n * 1e6) : n))
                    }}
                  />
                  <small style={{ color: 'var(--text-3)', minWidth: 28 }}>{isRp ? 'jt' : f.unit}</small>
                </span>
              </li>
            )
          })}
        </ul>
      </div>
      {(hist ?? []).length > 0 && (
        <div className="sec">
          <h4>Riwayat</h4>
          <ul className="ext">
            {(hist ?? []).map((h) => <li key={h.version}><Icon name="doc" /><span>Versi {h.version} · {shortDate(h.updated_at)} {hhmm(h.updated_at)}{h.updated_by_name ? ` · ${h.updated_by_name}` : ''}</span></li>)}
          </ul>
        </div>
      )}
      <div className="ft">
        <button className="btn primary" disabled={save.isPending} onClick={() => save.mutate({ key: policyKey, value: draft }, { onSuccess: closeSheet })}><Icon name="check" />Simpan</button>
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
        <span className="spacer" />
        <span className="pol"><Icon name="lock" />Divalidasi server · tercatat di audit</span>
      </div>
    </>
  )
}
