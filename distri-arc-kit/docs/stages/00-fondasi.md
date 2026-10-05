# Stage 00 — Fondasi: monorepo, Postgres, migrasi, seed, Makefile, CI
**Baca**: `docs/design/00-overview.md`, `02-architecture.md`, `03-data-model.md`, `10-testing-ops.md`, ADR 0001, 0005.

## Tujuan
Repo yang bisa dijalankan satu perintah: Postgres di Compose, migrasi goose, sqlc, seed 18 dealer contoh, API `health`, worker river yang hidup, web Vite kosong dengan token mockup, dan `make check` hijau di CI.

## Deliverables
1. Struktur: `cmd/arc`, `internal/{domain,metrics,store,api,events,policy}`, `db/{migrations,queries,seed}`, `web/`, `infra/`, `Makefile`, `.env.example`, `.golangci.yml`, `.github/workflows/check.yml`.
2. `go.mod` (Go 1.23) dengan dependensi inti: chi, pgx/v5, sqlc (tool), goose, river, slog (std), testcontainers-go (test).
3. Migrasi `0001_init.sql` memuat **semua tabel** di `03-data-model.md` (tanpa partisi dulu; `signals` dan `chat_messages` tabel biasa dengan catatan `-- partition at stage 13`), extension `pgcrypto`, `pg_trgm`. Migrasi `0002_river.sql` dari river.
4. `db/queries/*.sql` + `sqlc.yaml` → `internal/store/gen`. Repository tipis per agregat (dealers, signals, proposals, cycles, policies).
5. `db/seed/dealers.json`, `policies.json`, `sales.json`, `stock.json`, `signals.json` — data mockup (18 dealer, 4 sales, stok 4 cabang, ≥ 60 pesan WA sintetis). `arc ctl seed` idempoten (upsert by `source_id`).
6. `arc api`: `/api/health`, `/api/me` (auth sementara: header `X-Dev-User` hanya saat `APP_ENV=dev`), middleware log JSON + request id + recover.
7. `arc worker`: river client dengan periodic job `heartbeat` tiap menit (menulis `audit_log`), bukti worker hidup.
8. `web/`: Vite + React 19 + TS; `styles/tokens.css` disalin dari `:root` mockup (light + dark); halaman kosong "Distri ARC Orbit · Stage 00" memakai font dan token; proxy `/api`.
9. `Makefile`: `dev`, `check`, `test`, `lint`, `migrate`, `seed`, `sqlc`, `e2e` (placeholder). `infra/docker-compose.yml` (postgres 16 + volume).
10. `README.md`: cara jalan 5 langkah.

## Acceptance
- `make dev` → postgres up, migrasi jalan, seed terisi (`select count(*) from dealers` = 18), API `GET /api/health` 200 dengan `{db:"ok", queue:"ok"}`, worker menulis heartbeat, web tampil.
- `make check` hijau: `go vet`, `golangci-lint`, `go test ./...` (termasuk uji integrasi store dengan testcontainers atau `DATABASE_URL_TEST`), `npm run typecheck`, `npm test` (1 uji smoke).
- `arc ctl seed` dua kali → jumlah baris tidak berubah.
- CI workflow lolos pada push.

## Di luar scope
Rumus metrik, agen, WA, Odoo, UI layar.

Commit: `feat(stage-00): fondasi monorepo go+react+postgres, migrasi, seed, ci`
