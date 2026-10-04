// Bridge settings from the environment. Anti-ban limits are conservative by
// default and can only be loosened deliberately (see docs/adr/0004).
export interface Policy {
  maxPerHour: number        // sends per session in a sliding hour
  maxPerDay: number         // sends per session per calendar day (WIB)
  maxPerChatPerHour: number // sends to one chat in a sliding hour
  minGapSameChatMs: number  // minimum spacing between two sends to the same chat
  warmupDays: number        // after pairing, the daily cap ramps up over this many days (0 = off)
  warmupStartPerDay: number // daily cap on the first day after pairing
  quietStartHour: number    // no sends from this hour (WIB)…
  quietEndHour: number      // …until this hour (WIB)
  requirePriorInbound: boolean // only reply to chats that wrote to this number first
  maxSameTextChats: number  // identical text to more than N chats per hour = broadcast → refused
  minDelayMs: number        // random pause between consecutive sends of a session
  maxDelayMs: number
  typingMsPerChar: number   // "typing…" presence before each message
  typingMinMs: number
  typingMaxMs: number
  profileLookupsPerHour: number // contact/profile queries per session per hour
}

export interface Config {
  port: number
  secret: string
  apiUrl: string
  databaseUrl: string
  dataDir: string
  env: string
  policy: Policy
}

const num = (k: string, def: number): number => {
  const v = Number(process.env[k])
  return Number.isFinite(v) && process.env[k] !== '' && process.env[k] !== undefined ? v : def
}
const str = (k: string, def: string): string => (process.env[k]?.trim() ? process.env[k]!.trim() : def)

export const defaultPolicy: Policy = {
  maxPerHour: 20,
  maxPerDay: 120,
  maxPerChatPerHour: 6,
  minGapSameChatMs: 20_000,
  warmupDays: 7,
  warmupStartPerDay: 15,
  quietStartHour: 21,
  quietEndHour: 7,
  requirePriorInbound: true,
  maxSameTextChats: 3,
  minDelayMs: 2_000,
  maxDelayMs: 6_000,
  typingMsPerChar: 45,
  typingMinMs: 1_500,
  typingMaxMs: 8_000,
  profileLookupsPerHour: 30,
}

export function loadConfig(): Config {
  return {
    port: num('BRIDGE_PORT', 3001),
    secret: str('BRIDGE_SECRET', 'dev-bridge-secret'),
    apiUrl: str('ARC_API_URL', 'http://localhost:8000').replace(/\/+$/, ''),
    databaseUrl: str('BRIDGE_DATABASE_URL', str('DATABASE_URL', 'postgres://localhost:5433/arc?sslmode=disable')),
    dataDir: str('BRIDGE_DATA_DIR', './data'),
    env: str('ARC_ENV', 'development'),
    policy: {
      maxPerHour: num('BRIDGE_MAX_SEND_PER_HOUR', defaultPolicy.maxPerHour),
      maxPerDay: num('BRIDGE_MAX_SEND_PER_DAY', defaultPolicy.maxPerDay),
      maxPerChatPerHour: num('BRIDGE_MAX_SEND_PER_CHAT_HOUR', defaultPolicy.maxPerChatPerHour),
      minGapSameChatMs: num('BRIDGE_MIN_GAP_SAME_CHAT_SEC', defaultPolicy.minGapSameChatMs / 1000) * 1000,
      warmupDays: num('BRIDGE_WARMUP_DAYS', defaultPolicy.warmupDays),
      warmupStartPerDay: num('BRIDGE_WARMUP_START_PER_DAY', defaultPolicy.warmupStartPerDay),
      quietStartHour: num('BRIDGE_QUIET_START_HOUR', defaultPolicy.quietStartHour),
      quietEndHour: num('BRIDGE_QUIET_END_HOUR', defaultPolicy.quietEndHour),
      requirePriorInbound: str('BRIDGE_REQUIRE_PRIOR_INBOUND', 'on') !== 'off',
      maxSameTextChats: num('BRIDGE_MAX_SAME_TEXT_CHATS', defaultPolicy.maxSameTextChats),
      minDelayMs: defaultPolicy.minDelayMs,
      maxDelayMs: defaultPolicy.maxDelayMs,
      typingMsPerChar: defaultPolicy.typingMsPerChar,
      typingMinMs: defaultPolicy.typingMinMs,
      typingMaxMs: defaultPolicy.typingMaxMs,
      profileLookupsPerHour: num('BRIDGE_PROFILE_LOOKUPS_PER_HOUR', defaultPolicy.profileLookupsPerHour),
    },
  }
}
