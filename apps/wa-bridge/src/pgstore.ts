import pg from 'pg'
import { BufferJSON, initAuthCreds, proto, type AuthenticationCreds, type AuthenticationState, type SignalDataTypeMap } from 'baileys'
import { startOfWibDay, type SendFacts } from './guard.js'
import { textHash } from './sign.js'
import type { SentRecord, SessionMeta, Store } from './store.js'

// Everything lives in schema wa_bridge, so `arc reset` (which drops public) never unlinks phones.
const DDL = `
CREATE SCHEMA IF NOT EXISTS wa_bridge;
CREATE TABLE IF NOT EXISTS wa_bridge.arc_sessions (
  id text PRIMARY KEY, label text NOT NULL DEFAULT '', history_days int NOT NULL DEFAULT 30,
  paired_at timestamptz, phone text NOT NULL DEFAULT '', updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS wa_bridge.arc_auth (
  session text NOT NULL, key text NOT NULL, value text NOT NULL, PRIMARY KEY (session, key));
CREATE TABLE IF NOT EXISTS wa_bridge.arc_chats (
  session text NOT NULL, chat text NOT NULL, first_inbound_at timestamptz NOT NULL, opted_out boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (session, chat));
CREATE TABLE IF NOT EXISTS wa_bridge.arc_sends (
  id bigserial PRIMARY KEY, session text NOT NULL, chat text NOT NULL, action_id text NOT NULL, wamid text NOT NULL,
  text_hash text NOT NULL, at timestamptz NOT NULL DEFAULT now(), UNIQUE (session, action_id));
CREATE INDEX IF NOT EXISTS arc_sends_session_at ON wa_bridge.arc_sends (session, at);
`

export class PgStore implements Store {
  constructor(readonly pool: pg.Pool) {}

  async init(): Promise<void> { await this.pool.query(DDL) }

  async sessions(): Promise<SessionMeta[]> {
    const r = await this.pool.query(`SELECT id, label, history_days, paired_at, phone FROM wa_bridge.arc_sessions ORDER BY id`)
    return r.rows.map(x => ({ id: x.id, label: x.label, historyDays: x.history_days, pairedAt: x.paired_at, phone: x.phone }))
  }

  async upsertSession(m: SessionMeta): Promise<void> {
    await this.pool.query(`INSERT INTO wa_bridge.arc_sessions(id,label,history_days,paired_at,phone) VALUES ($1,$2,$3,$4,$5)
      ON CONFLICT (id) DO UPDATE SET label=EXCLUDED.label, history_days=EXCLUDED.history_days,
        paired_at=COALESCE(wa_bridge.arc_sessions.paired_at, EXCLUDED.paired_at), phone=EXCLUDED.phone, updated_at=now()`,
    [m.id, m.label, m.historyDays, m.pairedAt, m.phone])
  }

  async deleteSession(id: string): Promise<void> {
    await this.pool.query(`DELETE FROM wa_bridge.arc_auth WHERE session=$1`, [id])
    await this.pool.query(`DELETE FROM wa_bridge.arc_sessions WHERE id=$1`, [id])
  }

  async markInbound(session: string, chat: string, at: Date, optOut: boolean | null): Promise<void> {
    await this.pool.query(`INSERT INTO wa_bridge.arc_chats(session,chat,first_inbound_at,opted_out) VALUES ($1,$2,$3,COALESCE($4,false))
      ON CONFLICT (session,chat) DO UPDATE SET first_inbound_at=LEAST(wa_bridge.arc_chats.first_inbound_at, EXCLUDED.first_inbound_at),
        opted_out=COALESCE($4, wa_bridge.arc_chats.opted_out), updated_at=now()`, [session, chat, at, optOut])
  }

  async sentForAction(session: string, actionId: string): Promise<SentRecord | null> {
    const r = await this.pool.query(`SELECT chat, wamid, at, text_hash FROM wa_bridge.arc_sends WHERE session=$1 AND action_id=$2`, [session, actionId])
    if (!r.rows[0]) return null
    const x = r.rows[0]
    return { session, actionId, chat: x.chat, wamid: x.wamid, at: x.at, hash: x.text_hash }
  }

  async recordSend(s: SentRecord): Promise<void> {
    await this.pool.query(`INSERT INTO wa_bridge.arc_sends(session,chat,action_id,wamid,text_hash,at) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
      [s.session, s.chat, s.actionId, s.wamid, s.hash, s.at])
  }

  async facts(session: string, chat: string, text: string, now: Date): Promise<SendFacts> {
    const hourAgo = new Date(now.getTime() - 3_600_000)
    const r = await this.pool.query(`SELECT
        (SELECT paired_at FROM wa_bridge.arc_sessions WHERE id=$1) AS paired_at,
        (SELECT count(*)::int FROM wa_bridge.arc_sends WHERE session=$1 AND at > $3) AS last_hour,
        (SELECT count(*)::int FROM wa_bridge.arc_sends WHERE session=$1 AND at >= $4) AS today,
        (SELECT count(*)::int FROM wa_bridge.arc_sends WHERE session=$1 AND chat=$2 AND at > $3) AS chat_hour,
        (SELECT max(at) FROM wa_bridge.arc_sends WHERE session=$1 AND chat=$2) AS last_chat,
        (SELECT count(DISTINCT chat)::int FROM wa_bridge.arc_sends WHERE session=$1 AND chat<>$2 AND text_hash=$5 AND at > $3) AS same_text,
        (SELECT opted_out FROM wa_bridge.arc_chats WHERE session=$1 AND chat=$2) AS opted_out`,
    [session, chat, hourAgo, startOfWibDay(now), textHash(text)])
    const x = r.rows[0]
    return {
      now, pairedAt: x.paired_at, sentLastHour: x.last_hour, sentToday: x.today, sentToChatLastHour: x.chat_hour,
      lastSentToChatAt: x.last_chat, sameTextChatsLastHour: x.same_text,
      knownChat: x.opted_out !== null || chat.endsWith('@g.us'), optedOut: x.opted_out === true,
    }
  }

  async stats(session: string, now: Date): Promise<{ lastHour: number; today: number }> {
    const r = await this.pool.query(`SELECT count(*) FILTER (WHERE at > $2)::int AS h, count(*) FILTER (WHERE at >= $3)::int AS d FROM wa_bridge.arc_sends WHERE session=$1`,
      [session, new Date(now.getTime() - 3_600_000), startOfWibDay(now)])
    return { lastHour: r.rows[0].h, today: r.rows[0].d }
  }

  /** Baileys auth state backed by Postgres (the file-based helper is not meant for production). */
  async authState(session: string): Promise<{ state: AuthenticationState; saveCreds: () => Promise<void> }> {
    const read = async (key: string): Promise<unknown> => {
      const r = await this.pool.query(`SELECT value FROM wa_bridge.arc_auth WHERE session=$1 AND key=$2`, [session, key])
      return r.rows[0] ? JSON.parse(r.rows[0].value, BufferJSON.reviver) : null
    }
    const write = (key: string, value: unknown) => this.pool.query(
      `INSERT INTO wa_bridge.arc_auth(session,key,value) VALUES ($1,$2,$3) ON CONFLICT (session,key) DO UPDATE SET value=EXCLUDED.value`,
      [session, key, JSON.stringify(value, BufferJSON.replacer)])
    const creds = ((await read('creds')) as AuthenticationCreds | null) ?? initAuthCreds()
    return {
      state: {
        creds,
        keys: {
          get: async <T extends keyof SignalDataTypeMap>(type: T, ids: string[]) => {
            const out: { [id: string]: SignalDataTypeMap[T] } = {}
            if (!ids.length) return out
            const r = await this.pool.query(`SELECT key, value FROM wa_bridge.arc_auth WHERE session=$1 AND key = ANY($2)`, [session, ids.map(id => `${type}-${id}`)])
            for (const row of r.rows) {
              let v = JSON.parse(row.value, BufferJSON.reviver)
              if (type === 'app-state-sync-key' && v) v = proto.Message.AppStateSyncKeyData.fromObject(v)
              out[(row.key as string).slice(type.length + 1)] = v
            }
            return out
          },
          set: async data => {
            const client = await this.pool.connect()
            try {
              await client.query('BEGIN')
              for (const category in data) {
                const entries = data[category as keyof SignalDataTypeMap] ?? {}
                for (const id in entries) {
                  const value = entries[id]
                  const key = `${category}-${id}`
                  if (value) {
                    await client.query(`INSERT INTO wa_bridge.arc_auth(session,key,value) VALUES ($1,$2,$3) ON CONFLICT (session,key) DO UPDATE SET value=EXCLUDED.value`,
                      [session, key, JSON.stringify(value, BufferJSON.replacer)])
                  } else {
                    await client.query(`DELETE FROM wa_bridge.arc_auth WHERE session=$1 AND key=$2`, [session, key])
                  }
                }
              }
              await client.query('COMMIT')
            } catch (e) {
              await client.query('ROLLBACK')
              throw e
            } finally {
              client.release()
            }
          },
        },
      },
      saveCreds: async () => { await write('creds', creds) },
    }
  }
}
