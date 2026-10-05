# Changelog
Semua perubahan per tahap dicatat di sini oleh Claude Code (`feat(stage-NN): ...`).

## Stage 00 — Fondasi · 2026-10-05
- `cmd/arc` satu binary: `api` (chi, `/api/health`, `/api/me`), `worker` (river, heartbeat tiap menit → `audit_log`), `ctl` (`migrate`, `seed [--if-empty]`, `reset`, `counts`).
- Migrasi goose `0001_init.sql` (semua tabel `03-data-model.md` + `commitments`, view `v_dealer_board`) dan `0002_river.sql`.
- sqlc (`db/queries` → `internal/store/gen`), store + transaksi, `internal/testdb` (skema terisolasi per test).
- Seed 18 dealer, 4 sales + CEO/admin/finance, 379 SO, 379 invoice, 85 sinyal (68 WA), stok 4 cabang — digenerasi dari data contoh mockup (`tools/seedgen`) dan idempoten.
- `web/`: Vite + React 19 + TS, `tokens.css` + `mockup.css` disalin verbatim dari mockup, font self-host, proxy `/api`.
- `Makefile` (`dev`, `check`, `test`, `lint`, `migrate`, `seed`, `sqlc`, `seedgen`, `e2e`), `.golangci.yml`, CI `check.yml`, `infra/docker-compose.yml`, `.env.example`. ADR 0006.
