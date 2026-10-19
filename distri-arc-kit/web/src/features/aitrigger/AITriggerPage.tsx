import { useMemo, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { ReplyAction, ReplyEngine, ReplyRule, ReplyRules, ReplySuggestion } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useDealers, useMe } from '../../app/queries'
import { ACTION_ORDER, ACTIONS, parseKeywords, unknownPlaceholders } from './aitrigger'
import { TriggerCard } from './TriggerCard'

// AI Trigger (ADR 0038, ADR 0041): a customer message with one of a trigger's words gets a card from the data under
// its bubble in Chat — stock, price, credit, or order status — with a draft the template or an AI writes. The sales
// decides; nothing is sent automatically.

export const useReplyRules = () => useQuery({ queryKey: ['reply-rules'], queryFn: () => api.get<ReplyRules>('/reply-rules') })

type RuleBody = Omit<ReplyRule, 'id' | 'updated_by' | 'updated_at'>

const bodyOf = (r: ReplyRule): RuleBody => ({
  name: r.name, action: r.action, keywords: r.keywords, template_ready: r.template_ready, template_empty: r.template_empty,
  show_qty: r.show_qty, enabled: r.enabled, ai_phrase: r.ai_phrase, ai_instruction: r.ai_instruction,
})

function useSaveRule(onDone?: () => void) {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  return useMutation({
    mutationFn: ({ id, body }: { id?: string; body: RuleBody }) => (id ? api.put(`/reply-rules/${id}`, body) : api.post('/reply-rules', body)),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['reply-rules'] }); qc.invalidateQueries({ queryKey: ['chat'] }); onDone?.() },
    onError: (e: Error) => toast(e.message),
  })
}

function RuleSheet({ rule, aiReady }: { rule?: ReplyRule; aiReady: boolean }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const [action, setAction] = useState<ReplyAction>(rule?.action ?? 'stock')
  const d = ACTIONS[action].defaults
  const [name, setName] = useState(rule?.name ?? d.name)
  const [keywords, setKeywords] = useState((rule?.keywords ?? d.keywords).join(', '))
  const [ready, setReady] = useState(rule?.template_ready ?? d.template_ready)
  const [empty, setEmpty] = useState(rule?.template_empty ?? d.template_empty)
  const [showQty, setShowQty] = useState(rule?.show_qty ?? true)
  const [enabled, setEnabled] = useState(rule?.enabled ?? true)
  const [aiPhrase, setAIPhrase] = useState(rule?.ai_phrase ?? false)
  const [instr, setInstr] = useState(rule?.ai_instruction ?? '')
  const info = ACTIONS[action]
  const pickAction = (a: ReplyAction) => {
    setAction(a)
    if (!rule) { // a new trigger takes the defaults of its kind
      const x = ACTIONS[a].defaults
      setName(x.name); setKeywords(x.keywords.join(', ')); setReady(x.template_ready); setEmpty(x.template_empty)
    }
  }
  const save = useSaveRule(() => { toast('Trigger disimpan'); closeSheet() })
  const del = useMutation({
    mutationFn: () => api.del(`/reply-rules/${rule!.id}`),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['reply-rules'] }); toast('Trigger dihapus'); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const unknown = [...new Set([...unknownPlaceholders(ready, action), ...unknownPlaceholders(empty, action)])]
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate({ id: rule?.id, body: { name, action, keywords: parseKeywords(keywords), template_ready: ready, template_empty: empty, show_qty: showQty, enabled, ai_phrase: aiPhrase, ai_instruction: instr } })
  }
  return (
    <form onSubmit={submit}>
      <SheetHead icon="spark" title={rule ? `Ubah · ${rule.name}` : 'Trigger baru'} sub="Pesan pelanggan yang memuat salah satu kata kunci mendapat kartu data dan draf jawaban di Chat" onClose={closeSheet} />
      <div className="sec frm rr-form">
        <label className="full">Jenis data
          <select value={action} onChange={(e) => pickAction(e.target.value as ReplyAction)}>
            {ACTION_ORDER.map((a) => <option key={a} value={a}>{ACTIONS[a].label} — {ACTIONS[a].what}</option>)}
          </select>
        </label>
        <label>Nama trigger<input value={name} onChange={(e) => setName(e.target.value)} required /></label>
        <label>Kata kunci (pisahkan dengan koma)<input value={keywords} onChange={(e) => setKeywords(e.target.value)} placeholder={d.keywords.join(', ')} required /></label>
        <label className="full">{info.ready}<input value={ready} onChange={(e) => setReady(e.target.value)} required /></label>
        <label className="full">{info.empty}<input value={empty} onChange={(e) => setEmpty(e.target.value)} required /></label>
        <div className="full at-ph">
          {Object.entries(info.placeholders).map(([k, v]) => <span key={k}><code>{`{${k}}`}</code> {v}</span>)}
        </div>
        {unknown.length > 0 && <small className="full at-warn"><Icon name="alert" />Tidak diisi untuk jenis ini: {unknown.map((u) => `{${u}}`).join(', ')}</small>}
        {action === 'stock' && <label className="chk full"><input type="checkbox" checked={showQty} onChange={(e) => setShowQty(e.target.checked)} />Sebutkan jumlah stok di draf ke pelanggan (bila tidak: cukup "ready"). Sales tetap melihat jumlahnya.</label>}
        {action === 'credit' && <small className="full at-warn"><Icon name="lock" />Data kredit bersifat internal: kartu hanya tampil untuk dealer yang tertaut di Chat, dan sales memeriksa sebelum mengirim.</small>}
        <label className="chk full"><input type="checkbox" checked={aiPhrase} onChange={(e) => setAIPhrase(e.target.checked)} />
          <span><b>Jawaban ditulis AI</b> — AI merangkai draf yang luwes dari angka yang sama (angka tetap dihitung sistem; draf yang mengubah angka ditolak).{!aiReady && ' AI belum aktif: isi kunci di API & model AI.'}</span>
        </label>
        {aiPhrase && <label className="full">Instruksi untuk AI (opsional)<textarea rows={2} maxLength={500} value={instr} onChange={(e) => setInstr(e.target.value)} placeholder='Mis. "Sapa dengan Pak/Bu, tawarkan cek cabang lain bila kosong, tutup dengan terima kasih."' /></label>}
        <label className="chk full"><input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />Aktif</label>
      </div>
      <div className="ft">
        <button className="btn primary" type="submit" disabled={save.isPending}><Icon name="check" />Simpan</button>
        {rule && <button className="btn quiet" type="button" disabled={del.isPending} onClick={() => window.confirm(`Hapus trigger "${rule.name}"?`) && del.mutate()}>Hapus</button>}
        <span className="spacer" />
        <button className="btn quiet" type="button" onClick={closeSheet}>Batal</button>
      </div>
    </form>
  )
}

/** Try a customer sentence against the enabled triggers, for a branch or a dealer, optionally with the AI. */
function TryIt({ aiReady }: { aiReady: boolean }) {
  const [text, setText] = useState('Pak, kamera 4MP ready? harganya berapa?')
  const [before, setBefore] = useState('')
  const [branch, setBranch] = useState('')
  const [dealerQ, setDealerQ] = useState('')
  const [ai, setAI] = useState(false)
  const { data: dealers = [] } = useDealers(dealerQ.length >= 2 ? dealerQ : '')
  const dealer = useMemo(() => dealers.find((x) => x.name.toLowerCase() === dealerQ.trim().toLowerCase()), [dealers, dealerQ])
  const run = useMutation({
    mutationFn: () => api.post<{ suggestions: ReplySuggestion[]; dealer: string; branch: string; ai_error: string }>('/reply-rules/test', { text, branch, dealer: dealer?.id ?? '', ai, history: before.split('\n').map((l) => l.trim()).filter(Boolean) }),
  })
  const res = run.data
  return (
    <div className="card">
      <div className="card-h"><h2>Coba kalimat pelanggan</h2></div>
      <div className="at-try">
        <textarea rows={2} value={before} onChange={(e) => setBefore(e.target.value)} aria-label="Pesan sebelumnya" placeholder="Pesan sebelumnya (opsional, satu per baris) — mis. “Pak kamera 4MP harganya berapa?” lalu tanya “stoknya ada?” di bawah" />
        <textarea rows={2} value={text} onChange={(e) => setText(e.target.value)} aria-label="Kalimat pelanggan" />
        <div className="rr-try-row">
          <input value={dealerQ} onChange={(e) => setDealerQ(e.target.value)} list="at-dealers" placeholder="Dealer (opsional — untuk harga tier, limit, order)" aria-label="Dealer" />
          <datalist id="at-dealers">{dealers.slice(0, 20).map((x) => <option key={x.id} value={x.name}>{x.city}</option>)}</datalist>
          <input value={branch} onChange={(e) => setBranch(e.target.value)} placeholder="Cabang (opsional)" aria-label="Cabang" style={{ maxWidth: 170 }} />
        </div>
        <div className="rr-try-row">
          <label className="at-chk"><input type="checkbox" checked={ai} disabled={!aiReady} onChange={(e) => setAI(e.target.checked)} />Pakai AI untuk trigger yang mengaktifkan "Jawaban ditulis AI"</label>
          <span className="spacer" />
          <button className="btn primary" type="button" onClick={() => run.mutate()} disabled={run.isPending || !text.trim()}><Icon name="search" />{run.isPending ? 'Mencoba…' : 'Coba'}</button>
        </div>
        {dealerQ && !dealer && dealers.length > 0 && <small className="muted">Pilih nama dealer dari daftar.</small>}
      </div>
      {run.error && <p className="at-err">{(run.error as Error).message}</p>}
      {res && (
        <div className="at-res">
          <small className="muted">{res.suggestions.length === 0 ? 'Tidak ada trigger yang cocok — pesan ini tidak mendapat kartu.' : `${res.suggestions.length} kartu${res.dealer ? ` · ${res.dealer}` : ''}${res.branch ? ` · cabang ${res.branch}` : ''}`}</small>
          {res.ai_error && <small className="at-warn"><Icon name="alert" />{res.ai_error}</small>}
          {before.split('\n').map((l) => l.trim()).filter(Boolean).map((l, i) => <div key={i} className="m in at-bubble at-prev"><div className="b">{l}</div></div>)}
          <div className="m in at-bubble"><div className="b">{text}</div></div>
          {res.suggestions.map((s) => <TriggerCard key={s.rule_id} s={s} />)}
        </div>
      )}
    </div>
  )
}

function RuleRow({ r, edit, aiReady }: { r: ReplyRule; edit: boolean; aiReady: boolean }) {
  const { openSheet } = useFeedback()
  const toggle = useSaveRule()
  const info = ACTIONS[r.action] ?? ACTIONS.stock
  return (
    <li className={r.enabled ? '' : 'off'}>
      <span className="at-ic"><Icon name={info.icon} /></span>
      <div>
        <span className="rr-name">
          <b>{r.name}</b>
          <Pill tone="neutral">{info.label}</Pill>
          {r.ai_phrase && <Pill tone="good" icon="spark">Ditulis AI</Pill>}
        </span>
        <span className="rr-kw">{r.keywords.map((k) => <code key={k}>{k}</code>)}</span>
        <span className="rr-tpl">{info.ready.replace('Draf bila ', '')}: “{r.template_ready}”</span>
        <span className="rr-tpl">{info.empty.replace('Draf bila ', '')}: “{r.template_empty}”</span>
        {r.ai_phrase && r.ai_instruction && <span className="rr-tpl">Instruksi AI: {r.ai_instruction}</span>}
      </div>
      <div className="at-act">
        {edit ? (
          <>
            <label className="at-sw" title={r.enabled ? 'Matikan' : 'Aktifkan'}>
              <input type="checkbox" checked={r.enabled} disabled={toggle.isPending} onChange={(e) => toggle.mutate({ id: r.id, body: { ...bodyOf(r), enabled: e.target.checked } })} />
              <span>{r.enabled ? 'Aktif' : 'Mati'}</span>
            </label>
            <button className="btn quiet" onClick={() => openSheet(<RuleSheet rule={r} aiReady={aiReady} />)}>Ubah</button>
          </>
        ) : <Pill tone={r.enabled ? 'good' : 'neutral'}>{r.enabled ? 'Aktif' : 'Mati'}</Pill>}
      </div>
    </li>
  )
}

const ENGINE_LABEL: Record<string, string> = { openai: 'ChatGPT', claude: 'Claude' }
const MODEL_HINT: Record<string, string> = {
  'gpt-4.1-mini': 'cepat · ±Rp 15/draf (disarankan)', 'gpt-4.1-nano': 'paling cepat & murah · kalimat lebih sederhana', 'gpt-4o-mini': 'cepat · murah',
  'gpt-5-mini': 'lebih teliti · lebih lambat', 'claude-haiku-5-5': 'cepat · ±Rp 5/draf', 'claude-sonnet-5-5': 'paling teliti · ±Rp 130/draf',
}

/** The AI that writes the recommendations (ADR 0051): ChatGPT or Claude, a fast model; one choice for every trigger. */
function EngineCard({ data, edit }: { data?: ReplyRules; edit: boolean }) {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const save = useMutation({
    mutationFn: (e: ReplyEngine) => api.put('/reply-rules/engine', e),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['reply-rules'] }); toast('AI penulis rekomendasi disimpan') },
    onError: (e: Error) => toast(e.message),
  })
  if (!data?.engine) return null
  const e = data.engine
  const models = data.engine_models[e.provider] ?? []
  return (
    <div className="card at-engine">
      <div className="card-h">
        <h2>AI penulis rekomendasi</h2>
        {data.ai_ready ? <Pill tone="good" icon="check">Aktif · {ENGINE_LABEL[e.provider]}</Pill> : <Pill tone="warn" icon="alert">Kunci {ENGINE_LABEL[e.provider]} belum diisi</Pill>}
        <span className="meta">dipanggil hanya saat pesan cocok dengan trigger</span>
      </div>
      <div className="at-engine-row">
        <div className="seg" role="radiogroup" aria-label="AI">
          {(['openai', 'claude'] as const).map((p) => (
            <button key={p} role="radio" aria-checked={e.provider === p} className={e.provider === p ? 'is-active' : ''} disabled={!edit || save.isPending}
              onClick={() => save.mutate({ provider: p, model: data.engine_models[p]?.[0] ?? '' })}>{ENGINE_LABEL[p]}</button>
          ))}
        </div>
        <select className="cl-input" value={e.model} disabled={!edit || save.isPending} onChange={(x) => save.mutate({ provider: e.provider, model: x.target.value })} aria-label="Model">
          {models.map((m) => <option key={m} value={m}>{m} — {MODEL_HINT[m] ?? ''}</option>)}
        </select>
      </div>
      <p className="at-engine-hint">ChatGPT hanya dipakai di sini (rekomendasi jawaban dari angka sistem). Analisa Orchestrator dan Analisa WhatsApp tetap Claude. Kunci diisi di <a href="/pengaturan/api-ai">Pengaturan → API &amp; model AI</a>.</p>
    </div>
  )
}

export function AITriggerPage() {
  const nav = useNavigate()
  const { data: me } = useMe()
  const { data, isLoading } = useReplyRules()
  const { openSheet } = useFeedback()
  const edit = !!me && (me.screens.includes('users') || me.screens.includes('conn'))
  const rules = data?.items ?? []
  const aiReady = !!data?.ai_ready
  return (
    <div className="stack at">
      <div className="card">
        <div className="card-h">
          <h2>Cara kerja</h2>
          {aiReady ? <Pill tone="good" icon="spark">AI aktif</Pill> : <button className="btn quiet" style={{ marginLeft: 'auto' }} onClick={() => nav('/pengaturan/api-ai')}><Icon name="plug" />AI belum aktif — isi kunci API</button>}
        </div>
        <ol className="at-steps">
          <li><b>Pelanggan menulis</b> pesan WhatsApp yang memuat kata kunci trigger, mis. “stok”, “harga”, “tagihan”, “status order”.</li>
          <li><b>Sistem mengambil data</b> — stok per cabang, harga sesuai tier, sisa limit &amp; tagihan, atau status order — dan menampilkan kartunya (warna ungu AI, beda dari bubble) di bawah pesan itu di Chat. Bila pesan tidak menyebut produk (“stoknya ada?”), produk diambil dari 10 pesan sebelumnya.</li>
          <li><b>AI menulis rekomendasi jawaban</b> saat itu juga (1–2 detik) dengan membaca riwayat chat — hanya untuk pesan yang cocok dengan trigger, jadi hemat. Angka stok &amp; harga tetap dari sistem; draf yang angkanya tidak cocok ditolak dan draf template dipakai.</li>
          <li><b>Sales memutuskan</b>: tekan <i>Pakai jawaban</i>, edit, lalu kirim sendiri. Tidak ada yang terkirim otomatis.</li>
        </ol>
      </div>
      <EngineCard data={data} edit={edit} />
      <div className="card">
        <div className="card-h">
          <h2>Daftar trigger</h2>
          <span className="meta">{rules.filter((r) => r.enabled).length} aktif dari {rules.length}</span>
          {edit && <button className="btn primary" style={{ marginLeft: 'auto', height: 32, fontSize: 12.5 }} onClick={() => openSheet(<RuleSheet aiReady={aiReady} />)}><Icon name="check" />Trigger baru</button>}
        </div>
        {isLoading ? <p className="cl-empty">Memuat…</p> : rules.length === 0 ? <p className="cl-empty">Belum ada trigger.</p> : (
          <ul className="rr-list at-list">{rules.map((r) => <RuleRow key={r.id} r={r} edit={edit} aiReady={aiReady} />)}</ul>
        )}
        {!edit && <small className="muted"><Icon name="lock" /> Hanya CEO atau admin yang bisa mengubah trigger.</small>}
      </div>
      <TryIt aiReady={aiReady} />
    </div>
  )
}
