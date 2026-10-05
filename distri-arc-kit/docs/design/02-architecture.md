# 02 · Arsitektur — Go · React · PostgreSQL

## Topologi
```
┌──────────────┐   HTTPS   ┌────────────────────────────────────────────────────┐
│  Browser     │◄─────────►│  Caddy (TLS, static web/, reverse proxy /api /mcp)  │
│  React SPA   │  SSE      └───────────────┬────────────────────┬───────────────┘
└──────────────┘                            │                    │
                                   ┌────────▼───────┐    ┌───────▼───────┐
                                   │ cmd/api (Go)   │    │ cmd/worker(Go)│
                                   │ REST + SSE     │    │ river jobs:   │
                                   │ MCP server     │    │ orchestrator, │
                                   │ auth, policies │    │ wa ingest,    │
                                   └────────┬───────┘    │ odoo sync,    │
                                            │            │ outbox send   │
                                            │            └───────┬───────┘
                                   ┌────────▼────────────────────▼───────┐
                                   │ PostgreSQL 16 (data + river queue)  │
                                   └─────────────────────────────────────┘
          ┌──────────────┐  ┌───────────────┐  ┌──────────────┐  ┌──────────────┐
          │ WhatsApp     │  │ Odoo JSON-RPC │  │ LLM provider │  │ MCP clients  │
          │ whatsmeow /  │  │ (read, SO     │  │ Anthropic /  │  │ Claude, GPT, │
          │ Cloud API    │  │ draft write)  │  │ OpenAI       │  │ agent lain   │
          └──────────────┘  └───────────────┘  └──────────────┘  └──────────────┘
```
Satu binary `arc` dengan sub-perintah `api`, `worker`, `ctl`; Compose menjalankan dua proses (api, worker) dari image yang sama.

## Package Go
```
cmd/arc/main.go               # sub-perintah: api · worker · ctl (migrate, seed, reanalyze, mcp-token)
internal/domain               # tipe murni: Dealer, Signal, Proposal, Cycle, Policy… (tanpa DB/HTTP)
internal/metrics              # rumus Orbit (01-glossary) — fungsi murni + tabel uji
internal/store                # sqlc output + repository tipis (pgx), transaksi
internal/orchestrator         # Run(ctx, Scope, Trigger) → Cycle; 6 tahap; konflik; otonomi; belajar
internal/agents               # satu package per agen: order, followup, credit, stock, collect, prospect
internal/agents/agent.go      # interface Agent { Name(); Scope(); Analyze(ctx, Input) ([]Proposal, error) }
internal/llm                  # Provider interface; anthropic, openai, fake; prompt templates; PII mask; cost log
internal/wa                   # Transport interface; whatsmeow, cloudapi, fake; parser → signals; internal numbers
internal/odoo                 # Client JSON-RPC; sync SO/invoice/payment/stock/partner; SO draft write
internal/mcp                  # MCP server: tools, auth (token+scope), policies, audit
internal/api                  # chi routes, handlers, SSE hub, auth (session JWT), RBAC
internal/events               # bus in-process (Postgres LISTEN/NOTIFY) → SSE + worker
internal/policy               # loader/cache kebijakan (JSONB) + validasi + versi
db/migrations                 # goose SQL
db/queries                    # sqlc .sql
db/seed                       # 18 dealer contoh (JSON) + sinyal contoh
web/                          # React SPA (08-frontend.md)
infra/                        # docker-compose.yml, Caddyfile, systemd contoh
```

## Aliran data
1. **Ingest** (worker, kontinu): `wa` menerima pesan → `signals(kind='wa', payload JSONB, dedupe wa_msg_id)`; `odoo` sync tiap 10 menit (`write_date > last`) → `orders/invoices/payments/stock_items` + `signals(kind='so'|'invoice'|'payment'|'stock')`. Setiap sinyal memicu NOTIFY `signal_new`.
2. **Metrics** (worker, tiap sinyal relevan + tiap siklus): `metrics.Compute(dealer, history)` → `dealer_metrics_daily` (snapshot per hari) dan `dealers.metrics_current` (JSONB cache). UI membaca cache; riwayat untuk "3 bulan lalu" dari snapshot.
3. **Orchestrator** (worker): job `cycle.run` tiap jam 06–20 WIB + on-demand (`scope`) + MCP. Detail di `04-orchestrator.md`.
4. **Keputusan** (api): `POST /proposals/{id}/decide` → status `approved|edited|rejected` + `decided_by` + `reason` → bila approved: tulis `outbox` (pesan WA / SO draft / perubahan limit yang diminta ke Odoo manual) → worker `outbox.send` lewat `wa.Transport` atau `odoo.Client`.
5. **Belajar**: penolakan dengan alasan → `calibration_events` → aturan supresi 14 hari per (agent, dealer, jenis) + penyesuaian `agent_confidence`.
6. **Realtime**: Postgres NOTIFY (`cycle_stage`, `proposal_changed`, `chat_message`) → `events` → SSE `/api/events` ke browser; frontend meng-invalidate query terkait.

## Batas tanggung jawab
| Komponen | Boleh | Tidak boleh |
|---|---|---|
| Agen | membaca `Input` yang disiapkan Orchestrator, memanggil LLM, mengembalikan `Proposal` berprovenance | menulis DB, memanggil WA/Odoo, memanggil agen lain |
| Orchestrator | memanggil agen, menulis cycle/conflict/proposal/plan, memutuskan otonomi dari policy | mengirim ke dealer tanpa status `approved` |
| MCP server | tool baca/analisis/orkestrasi sesuai scope token | `actions.decide` untuk klien non-manusia; `outbox` |
| API | CRUD, keputusan manusia, SSE | memanggil agen langsung |

## Keandalan
- Semua job `river` idempoten dengan `unique_opts` (mis. `cycle.run` per jam, `outbox.send` per outbox id).
- Timeout LLM 60 dtk per agen, retry 2× backoff; agen gagal → tahap Analisis ditandai `partial`, siklus tetap selesai.
- `signals` dan `chat_messages` dipartisi bulanan; retensi chat 90 hari (job `retention.purge` harian).
- Backup: `pg_dump` harian ke objek storage (Stage 13).

## Observabilitas
`slog` JSON dengan `cycle_id`, `agent`, `dealer_id`; tabel `llm_calls` (provider, model, token in/out, biaya, hash prompt, durasi); endpoint `/api/health` (db, wa transport, odoo, llm) dan `/metrics` Prometheus opsional.
