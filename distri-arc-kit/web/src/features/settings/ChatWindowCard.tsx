import { useQueryClient } from '@tanstack/react-query'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useMe, usePolicies } from '../../app/queries'
import { useSavePolicy } from './PolicyEditor'

// chat.rules as seeded (db/seed/policies.json): the whole value is saved, so a database without the row still saves.
const SEED: Record<string, unknown> = { work_days: [0, 1, 2, 3, 4, 5, 6], work_start: '06:00', work_end: '22:00', reply_sla_min: 30, escalate_hours: 24 }

/** Pengaturan → Chat WhatsApp (CEO): keep WhatsApp's 24-hour rule in Chat (ADR 0081) or open Chat to write to a
 * customer any time (ADR 0082). */
export function ChatWindowCard() {
  const { data: pol } = usePolicies()
  const { data: me } = useMe()
  const { toast } = useFeedback()
  const save = useSavePolicy()
  const qc = useQueryClient()
  if (!pol) return null
  const rules = (pol['chat.rules']?.value as Record<string, unknown> | undefined) ?? SEED
  const open = rules.reply_window_open === true
  const set = (v: boolean) => {
    if (!me?.edit_policies) return toast('Hanya CEO yang mengubah aturan chat')
    if (v !== open) save.mutate({ key: 'chat.rules', value: { ...rules, reply_window_open: v } }, { onSuccess: () => qc.invalidateQueries({ queryKey: ['chat'] }) })
  }
  return (
    <div className="card">
      <div className="card-h"><h2>Chat WhatsApp</h2>{open ? <Pill tone="warn" icon="alert">Chat dibuka</Pill> : <Pill tone="good" icon="shield">Aturan 24 jam</Pill>}</div>
      <ul className="rules">
        <li>
          <div>
            <b>Batas balas 24 jam</b>
            <span>
              {open
                ? 'Dibuka: pelanggan bisa dikirimi pesan dari Chat kapan saja, termasuk yang belum pernah menulis. Risiko nomor diblokir WhatsApp lebih tinggi.'
                : 'Pelanggan hanya bisa dibalas dari Chat dalam 24 jam setelah mereka menulis, seperti aturan WhatsApp, agar nomor tidak diblokir. Di luar itu, chat dimulai dari WhatsApp Web atau HP.'}
            </span>
          </div>
          <div className="seg">
            <button className={open ? '' : 'is-active'} disabled={save.isPending} onClick={() => set(false)}>24 jam</button>
            <button className={open ? 'is-active' : ''} disabled={save.isPending} onClick={() => set(true)}>Dibuka</button>
          </div>
        </li>
      </ul>
      <p style={{ fontSize: 12, color: 'var(--text-3)', margin: '10px 0 0', lineHeight: 1.5 }}>Berlaku untuk chat pelanggan satu-satu di semua nomor. Grup dan chat internal tidak terikat aturan ini. Setiap perubahan tercatat sebagai versi kebijakan baru.</p>
    </div>
  )
}
