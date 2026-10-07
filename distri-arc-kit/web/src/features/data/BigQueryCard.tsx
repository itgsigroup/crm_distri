import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { api } from '../../api/client'
import type { DataStatus } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { useMe } from '../../app/queries'
import { useDataStatus } from './DataPage'

/** Uploads a Google service-account key (JSON). The key is sealed on the server; the project id comes from it. */
export function useKeyUpload() {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [busy, setBusy] = useState(false)
  const upload = async (f: File, st: DataStatus) => {
    setBusy(true)
    try {
      const r = await fetch('/api/data/credentials', { method: 'PUT', credentials: 'same-origin', body: await f.text(), headers: { 'Content-Type': 'application/json' } })
      const d = await r.json()
      if (!r.ok) throw new Error(d?.error?.message ?? 'Kunci tidak diterima')
      // BigQuery becomes the source, with the key's project unless one is set already
      const bq = { ...st.source.bigquery, project_id: st.source.bigquery.project_id || d.project_id }
      await api.put('/data/source', { ...st.source, mode: 'bigquery', bigquery: bq })
      toast(`Kunci ${d.client_email} tersimpan terenkripsi · project ${bq.project_id}`)
      qc.invalidateQueries({ queryKey: ['data'] })
    } catch (e) {
      toast((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return { upload, busy }
}

/** Pengaturan: the real-data connection (BigQuery · Accurate) — upload the key here, then map and sync in Data & master. */
export function BigQueryCard() {
  const { data: me } = useMe()
  const { data: st } = useDataStatus()
  const { upload, busy } = useKeyUpload()
  if (!st) return null
  const ceo = !!me?.edit_policies
  const cred = st.credentials
  const ok = cred.set && !cred.error
  const queries = Object.values(st.source.bigquery.queries ?? {}).filter((q) => q && q.trim()).length
  const last = st.runs.find((r) => r.entity === 'apply')
  const unmapped = st.mappings.reduce((n, m) => n + m.unmapped, 0)
  const pick = (
    <label className={`btn ${ok ? 'quiet' : 'primary'}`} style={{ height: 32, fontSize: 12.5 }} aria-disabled={busy}>
      <Icon name="lock" />{busy ? 'Mengunggah…' : ok ? 'Ganti kunci JSON' : 'Unggah kunci JSON'}
      <input type="file" accept=".json,application/json" hidden disabled={busy} onChange={(e) => { const f = e.target.files?.[0]; if (f) upload(f, st); e.target.value = '' }} />
    </label>
  )
  return (
    <div className="card bq-card">
      <div className="card-h">
        <span className="lg bq-lg">BQ</span>
        <h2>Data asli · BigQuery (Accurate)</h2>
        <Pill tone={ok ? 'good' : cred.error ? 'bad' : 'warn'}>{ok ? 'Kunci terpasang' : cred.error ? 'Kunci bermasalah' : 'Belum terhubung'}</Pill>
      </div>
      {ok ? (
        <ul className="rules">
          <li><div><b>{cred.client_email}</b><span>Project {st.source.bigquery.project_id || cred.project_id} · baca saja · kunci terenkripsi di server</span></div></li>
          <li><div><b>{queries}/5 jenis data dipetakan · {st.dealers} pelanggan · {st.invoices} faktur</b>
            <span>{last ? `Sinkron terakhir ${shortDate(last.started_at)} ${hhmm(last.started_at)}` : 'Belum sinkron'}{unmapped ? ` · ${unmapped} nilai belum dipetakan` : ''}{st.source.mode === 'bigquery' ? ` · otomatis tiap ${st.source.bigquery.sync_minutes} menit` : ''}</span></div></li>
        </ul>
      ) : cred.error ? (
        <p className="bq-note bad">{cred.error}</p>
      ) : (
        <ol className="bq-steps">
          <li>Google Cloud → <b>IAM &amp; Admin → Service accounts</b> → pilih akun (mis. <span className="mono">distri-data</span>) dengan peran <b>BigQuery Data Viewer</b> + <b>BigQuery Job User</b></li>
          <li>Tab <b>Keys → Add key → Create new key → JSON</b> — file .json terunduh</li>
          <li>Unggah file itu di sini; Project ID terisi otomatis dari kunci</li>
        </ol>
      )}
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center', marginTop: 10 }}>
        {ceo ? pick : !ok && <span style={{ fontSize: 12.5, color: 'var(--text-3)' }}>Kunci diunggah oleh pemegang hak kebijakan (CEO)</span>}
        <Link className={`btn ${ok ? 'primary' : 'ghost'}`} style={{ height: 32, fontSize: 12.5 }} to="/pengaturan/data">{ok && !queries ? 'Lanjut: petakan tabel Accurate' : 'Data & master'}<Icon name="arrow" /></Link>
      </div>
    </div>
  )
}
