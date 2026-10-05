# 10 · Pengujian, Seed, Operasi

## Piramida uji
| Lapisan | Alat | Wajib |
|---|---|---|
| Domain & metrics | `go test` tabel kasus | semua kasus di `01-glossary.md` |
| Agen | `go test` + `llm.Fake` (jawaban deterministik dari fixture JSON) | kasus di `05-agents.md` |
| Orchestrator | `go test` + Postgres (testcontainers) | kasus di `04-orchestrator.md` |
| Store/API | `go test` integrasi (httptest + Postgres) | setiap endpoint: happy path + authz |
| MCP | `go test` dengan klien SDK | kasus di `06-mcp.md` |
| Frontend | vitest | kasus di `08-frontend.md` |
| E2E | playwright terhadap `make dev` + seed | alur di `08-frontend.md` |

`make check` menjalankan semuanya kecuali e2e; `make e2e` untuk playwright. CI (GitHub Actions) menjalankan `make check` di setiap PR dengan service Postgres.

## Seed (`arc ctl seed`)
- 4 sales (Andi Semarang, Dewi Yogyakarta, Rizky Surabaya, Fajar Jakarta) + 1 CEO + admin.
- 18 dealer dengan order sintetis 6 bulan, invoice, pembayaran, kontak, memo, komitmen, timeline, sinyal WA contoh (≥ 60 pesan), 2 grup (gudang internal, 1 eksternal proyek), 1 nomor baru (Toko Mandiri Pati), stok 4 cabang dengan 3 SKU menua + 3 kritis.
- Setelah seed, `metrics.Compute` harus menghasilkan nilai mockup (lihat tabel di `01-glossary.md`) — ini **uji regresi seed** (`internal/store/seed_test.go`).
- Kebijakan default dari `db/seed/policies.json`.

## Fake & stub
- `llm.Fake`: memetakan `purpose+hash(input)` → fixture; bila tidak ada fixture, mengembalikan template deterministik (bukan error) supaya siklus tetap lengkap.
- `wa.Fake`: in-memory transport; `Send` mencatat ke `outbox` sebagai `sent`; dapat menyuntik pesan masuk untuk uji.
- `odoo.Fake`: membaca `db/seed/odoo/*.json` sebagai hasil RPC.

## Operasi
- `make dev`: compose up postgres → migrate → seed (bila kosong) → api :8080 → worker → web :5173 (proxy `/api`,`/mcp`).
- Produksi: `infra/docker-compose.prod.yml` (postgres, api, worker, caddy); `Caddyfile` melayani `web/dist`, proxy `/api`, `/mcp`, `/events`.
- Health: `/api/health` memeriksa db, wa transport, odoo (ping), llm (dry), river queue depth.
- Log: JSON ke stdout; `cycle_id` di setiap baris siklus.
- Backup: `infra/backup.sh` (pg_dump + age/gpg) harian 02:00; `infra/restore.sh` diuji.
- Runbook `docs/RUNBOOK.md` (Stage 13): WA terputus, Odoo gagal sync, LLM gagal, antrean menumpuk, rollback migrasi.
