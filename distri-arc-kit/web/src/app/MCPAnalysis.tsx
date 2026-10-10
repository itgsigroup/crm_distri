import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Cycle } from '../api/types'
import { Icon } from '../components/Icon'
import { SheetHead, useFeedback } from '../components/feedback'
import { Pill } from '../components/ui'
import { hhmm } from '../lib/format'
import { useOrchStatus } from './orch'

/** How a cycle asked from the "Analisis ulang" button is analysed by Claude through MCP (POST /cycles → mcp). */
export interface CycleMCP { engine: 'claude_api' | 'claude_ai'; prompt: string; claude_url: string; wait_sec: number; agents: string[] }
export type QueuedCycle = Cycle & { mcp?: CycleMCP }

interface InputState { agent: string; submitted_at: string | null }

const waitText = (sec: number) => (sec >= 60 ? `${Math.round(sec / 60)} menit` : `${sec} detik`)

/** "Claude menganalisis lewat MCP": open Claude with the task pre-filled (or watch the server's Claude), and
 * follow each agent until Claude has sent its analysis — the proposals land in GSI Orbit. */
export function MCPAnalysisSheet({ cycle }: { cycle: QueuedCycle }) {
  const { closeSheet, toast } = useFeedback()
  const orch = useOrchStatus()
  const m = cycle.mcp!
  const running = orch.running
  const { data: inputs = [] } = useQuery({
    queryKey: ['cycle', cycle.id, 'inputs'],
    queryFn: () => api.get<{ items: InputState[] }>(`/cycles/${cycle.id}/inputs`).then((r) => r.items),
    refetchInterval: running ? 3000 : false,
  })
  const sent = new Map(inputs.filter((x) => x.submitted_at).map((x) => [x.agent, x.submitted_at!]))
  const published = inputs.length > 0
  const copy = () => navigator.clipboard?.writeText(m.prompt).then(() => toast('Perintah disalin — tempel di Claude'), () => toast('Gagal menyalin; pilih teks lalu salin manual'))
  return (
    <>
      <SheetHead icon="plug" title={`Claude menganalisis lewat MCP · siklus #${cycle.number ?? ''}`}
        sub={m.engine === 'claude_api' ? 'Claude di server mengerjakan sendiri lewat tool MCP — hasilnya langsung tersimpan di GSI Orbit' : `Buka Claude dan kirim perintahnya — batas ${waitText(m.wait_sec)}; agen tanpa jawaban dicatat Gagal`} onClose={closeSheet} />
      <div className="sec mcpa">
        {m.engine === 'claude_ai' ? (
          <ol className="mcpa-steps">
            <li><b>Buka Claude</b> (akun yang konektor GSI Orbit-nya aktif) — perintahnya sudah terisi.
              <a className="btn primary" href={m.claude_url} target="_blank" rel="noopener noreferrer"><Icon name="plug" />Buka Claude</a></li>
            <li><b>Tekan Enter.</b> Claude mengambil data tiap agen (<code>orchestrator_input_get</code>), menganalisis, lalu mengirim usulannya (<code>orchestrator_submit</code>).</li>
            <li><b>Hasilnya tersimpan di GSI Orbit</b> — Keputusan, Rencana, dan Orchestrator ter-update sendiri. Halaman ini boleh ditutup.</li>
          </ol>
        ) : (
          <p className="mcpa-note"><Icon name="check" />Kunci Claude API tersimpan: Claude di server menjalankan analisis lewat tool MCP yang sama (tercatat di Riwayat MCP dan Analisis terjadwal). Tidak perlu membuka apa pun.</p>
        )}
        <div className="mcpa-agents">
          <div className="mcpa-h">Agen {sent.size}/{m.agents.length} dianalisis Claude{running ? '' : ' · siklus selesai'}</div>
          <ul>
            {m.agents.map((a) => {
              const at = sent.get(a)
              return (
                <li key={a}>
                  <span>{a}</span>
                  {at ? <Pill tone="good" icon="check">dikirim Claude {hhmm(at)}</Pill>
                    : running ? <Pill tone="accent">{published ? 'menunggu Claude…' : 'menyiapkan data…'}</Pill>
                    : <Pill tone="bad">Gagal · tanpa jawaban</Pill>}
                </li>
              )
            })}
          </ul>
        </div>
        {m.engine === 'claude_ai' && (
          <details className="mcpa-prompt">
            <summary>Lihat / salin perintah untuk Claude</summary>
            <pre>{m.prompt}</pre>
            <button type="button" className="btn ghost" onClick={copy}><Icon name="doc" />Salin perintah</button>
          </details>
        )}
      </div>
      <div className="ft"><button className="btn quiet" onClick={closeSheet}>Tutup</button><span className="spacer" /><span className="pol"><Icon name="lock" />Claude hanya mengusulkan · keputusan tetap di aplikasi</span></div>
    </>
  )
}
