import { textHash } from './sign.js'
import { startOfWibDay, type SendFacts } from './guard.js'

export interface SessionMeta {
  id: string
  label: string
  historyDays: number
  pairedAt: Date | null
  phone: string
}

export interface SentRecord {
  session: string
  chat: string
  actionId: string
  wamid: string
  at: Date
  hash: string
}

/** Persistence for sessions, send log, known chats and opt-outs (content is never stored). */
export interface Store {
  init(): Promise<void>
  sessions(): Promise<SessionMeta[]>
  upsertSession(m: SessionMeta): Promise<void>
  deleteSession(id: string): Promise<void>
  markInbound(session: string, chat: string, at: Date, optOut: boolean | null): Promise<void>
  sentForAction(session: string, actionId: string): Promise<SentRecord | null>
  recordSend(r: SentRecord): Promise<void>
  facts(session: string, chat: string, text: string, now: Date): Promise<SendFacts>
  stats(session: string, now: Date): Promise<{ lastHour: number; today: number }>
}

interface ChatState { firstInbound: Date; optedOut: boolean }

/** In-memory store for tests and local experiments. */
export class MemoryStore implements Store {
  private meta = new Map<string, SessionMeta>()
  private chats = new Map<string, ChatState>()
  sent: SentRecord[] = []

  async init(): Promise<void> {}
  async sessions(): Promise<SessionMeta[]> { return [...this.meta.values()] }
  async upsertSession(m: SessionMeta): Promise<void> { this.meta.set(m.id, { ...m }) }
  async deleteSession(id: string): Promise<void> { this.meta.delete(id) }

  async markInbound(session: string, chat: string, at: Date, optOut: boolean | null): Promise<void> {
    const k = session + '|' + chat
    const cur = this.chats.get(k)
    this.chats.set(k, { firstInbound: cur?.firstInbound ?? at, optedOut: optOut === null ? (cur?.optedOut ?? false) : optOut })
  }

  async sentForAction(session: string, actionId: string): Promise<SentRecord | null> {
    return this.sent.find(s => s.session === session && s.actionId === actionId) ?? null
  }

  async recordSend(r: SentRecord): Promise<void> { this.sent.push(r) }

  async facts(session: string, chat: string, text: string, now: Date): Promise<SendFacts> {
    const hourAgo = now.getTime() - 3_600_000
    const day = startOfWibDay(now).getTime()
    const mine = this.sent.filter(s => s.session === session)
    const toChat = mine.filter(s => s.chat === chat)
    const h = textHash(text)
    const st = this.chats.get(session + '|' + chat)
    const last = toChat.reduce<Date | null>((m, s) => (!m || s.at > m ? s.at : m), null)
    return {
      now,
      pairedAt: this.meta.get(session)?.pairedAt ?? null,
      sentLastHour: mine.filter(s => s.at.getTime() > hourAgo).length,
      sentToday: mine.filter(s => s.at.getTime() >= day).length,
      sentToChatLastHour: toChat.filter(s => s.at.getTime() > hourAgo).length,
      lastSentToChatAt: last,
      sameTextChatsLastHour: new Set(mine.filter(s => s.hash === h && s.chat !== chat && s.at.getTime() > hourAgo).map(s => s.chat)).size,
      knownChat: !!st || chat.endsWith('@g.us'),
      optedOut: st?.optedOut ?? false,
    }
  }

  async stats(session: string, now: Date): Promise<{ lastHour: number; today: number }> {
    const f = await this.facts(session, '', '', now)
    return { lastHour: f.sentLastHour, today: f.sentToday }
  }
}
