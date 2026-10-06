// Distri ARC wa-bridge (docs/adr/0017): links many WhatsApp numbers as companion devices via Baileys,
// forwards every message to the Distri ARC worker as a WaEvent, and sends a message only after the worker
// confirms the outbox row belongs to a human-approved proposal — within the anti-ban limits.
import { mkdir } from 'node:fs/promises'
import pg from 'pg'
import pino from 'pino'
import { loadConfig } from './config.js'
import { Forwarder } from './forward.js'
import { serve } from './http.js'
import { PgStore } from './pgstore.js'
import { Sender } from './sender.js'
import { Manager } from './session.js'

const cfg = loadConfig()
const log = pino({ level: process.env.BRIDGE_LOG_LEVEL ?? 'info', base: { svc: 'wa-bridge' } })
// Baileys is chatty at info level; keep its internals at warn.
const waLog = log.child({ mod: 'baileys' }, { level: process.env.BAILEYS_LOG_LEVEL ?? 'warn' })

if (cfg.env !== 'dev' && (cfg.secret === 'dev-bridge-secret' || cfg.secret.length < 16)) {
  log.fatal('BRIDGE_SECRET must be set (≥ 16 chars) in production')
  process.exit(1)
}

await mkdir(cfg.dataDir, { recursive: true, mode: 0o700 })
const pool = new pg.Pool({ connectionString: cfg.databaseUrl, max: 5 })
const store = new PgStore(pool)
await store.init()

const fwd = new Forwarder(cfg.apiUrl, cfg.secret, cfg.dataDir, log)
const retry = fwd.start()
const manager = new Manager(store, fwd, cfg.policy, waLog, cfg.deviceName)
const sender = new Sender(store, fwd, cfg.policy)
await manager.restore()

const server = serve({ secret: cfg.secret, manager, sender, store, policy: cfg.policy, queueLength: () => fwd.queueLength(), lastEvent: () => fwd.lastEvent }, cfg.port, cfg.host)
log.info({ port: cfg.port, host: cfg.host, api: cfg.apiUrl, policy: cfg.policy }, 'wa-bridge listening')

const shutdown = async () => {
  clearInterval(retry)
  server.close()
  await manager.stopAll()
  await pool.end()
  process.exit(0)
}
process.on('SIGINT', () => void shutdown())
process.on('SIGTERM', () => void shutdown())
