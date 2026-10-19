import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { PasswordInput } from '../../components/PasswordInput'
import { Pill } from '../../components/ui'

// Routine Claude (ADR 0049): a cycle waiting for Claude through MCP fires this routine on claude.ai, so Claude analyses
// in the cloud on the subscription — nobody opens Claude Desktop, no API key, no ChatGPT.

interface RoutineInfo { set: boolean; routine: string; url: string; can_configure: boolean }

/** The routine created for GSI Orbit on 19 Oct ("GSI Orbit · Analisis Orchestrator (dipicu server)"). */
const CREATED = 'trig_01VvHFKdz9UQj3aHMyoQ9isk'

export function ClaudeRoutine() {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const { data: info } = useQuery({ queryKey: ['ai', 'routine'], queryFn: () => api.get<RoutineInfo>('/ai/routine') })
  const [id, setId] = useState(CREATED)
  const [token, setToken] = useState('')
  const refresh = () => qc.invalidateQueries({ queryKey: ['ai', 'routine'] })
  const save = useMutation({
    mutationFn: () => api.put<RoutineInfo>('/ai/routine', { routine_id: id.trim(), token: token.trim() }),
    onSuccess: () => { setToken(''); refresh(); toast('Routine Claude disimpan — siklus berikutnya dianalisis otomatis') },
    onError: (e: Error) => toast(e.message),
  })
  const test = useMutation({
    mutationFn: () => api.post<{ url: string }>('/ai/routine/test'),
    onSuccess: (r) => { toast('Routine Claude berjalan — buka tautannya untuk melihat'); if (r.url) window.open(r.url, '_blank', 'noopener') },
    onError: (e: Error) => toast(e.message),
  })
  const remove = useMutation({ mutationFn: () => api.del('/ai/routine'), onSuccess: () => { refresh(); toast('Routine Claude dilepas') }, onError: (e: Error) => toast(e.message) })
  if (!info) return null
  const edit = info.can_configure
  return (
    <div className="card cr-card">
      <div className="card-h">
        <h2>Routine Claude · tanpa membuka Desktop</h2>
        {info.set ? <Pill tone="good" icon="check">Aktif</Pill> : <Pill tone="warn" icon="alert">Belum diisi</Pill>}
        <span className="meta">Claude lewat MCP, berjalan di cloud Claude · tanpa kunci API · tanpa ChatGPT</span>
      </div>
      <p className="cr-text">Setiap siklus yang menunggu Claude — tombol <b>Analisis ulang</b> dan siklus per jam — memicu routine ini di claude.ai. Claude membaca data lewat konektor CRM-DISTRI dan mengirim analisanya kembali, memakai langganan Claude. Tidak ada yang perlu membuka Claude Desktop.</p>
      {info.set ? (
        <div className="cr-row">
          <span>Routine <a href={info.url} target="_blank" rel="noopener noreferrer"><code>{info.routine}</code></a></span>
          {edit && <>
            <button className="btn ghost" disabled={test.isPending} onClick={() => test.mutate()}><Icon name="refresh" />Tes sekarang</button>
            <button className="btn quiet" disabled={remove.isPending} onClick={() => { if (window.confirm('Lepas routine Claude? Analisis ulang kembali harus dibuka di Claude.')) remove.mutate() }}>Lepas</button>
          </>}
        </div>
      ) : null}
      {edit && (
        <>
          <ol className="cr-steps">
            <li>Buka <a href={`https://claude.ai/code/routines/${CREATED}`} target="_blank" rel="noopener noreferrer">routine "GSI Orbit · Analisis Orchestrator"</a> di claude.ai → menu di samping nama → <b>Edit</b>.</li>
            <li><b>Add another trigger</b> → <b>API</b> → simpan, lalu klik <b>Generate token</b> dan salin tokennya (hanya ditampilkan sekali).</li>
            <li>Tempel di bawah, lalu <b>Simpan</b>. Tekan <b>Tes sekarang</b> untuk memastikan routine berjalan.</li>
          </ol>
          <div className="cr-form">
            <input className="cl-input" value={id} onChange={(e) => setId(e.target.value)} placeholder="trig_…" aria-label="ID routine" />
            <PasswordInput className="cl-input" value={token} onChange={(e) => setToken(e.target.value)} placeholder={info.set ? 'Token baru (sk-ant-oat01-…) untuk mengganti' : 'Token trigger API (sk-ant-oat01-…)'} aria-label="Token routine" autoComplete="off" />
            <button className="btn primary" disabled={!id.trim() || !token.trim() || save.isPending} onClick={() => save.mutate()}><Icon name="lock" />Simpan</button>
          </div>
        </>
      )}
      {!edit && !info.set && <p className="sg-hint">Diisi oleh CEO di halaman ini.</p>}
    </div>
  )
}
