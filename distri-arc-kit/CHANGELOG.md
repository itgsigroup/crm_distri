# Changelog
Semua perubahan per tahap dicatat di sini oleh Claude Code (`feat(stage-NN): ...`).

## Stage 00 — Fondasi · 2026-10-05
- `cmd/arc` satu binary: `api` (chi, `/api/health`, `/api/me`), `worker` (river, heartbeat tiap menit → `audit_log`), `ctl` (`migrate`, `seed [--if-empty]`, `reset`, `counts`).
- Migrasi goose `0001_init.sql` (semua tabel `03-data-model.md` + `commitments`, view `v_dealer_board`) dan `0002_river.sql`.
- sqlc (`db/queries` → `internal/store/gen`), store + transaksi, `internal/testdb` (skema terisolasi per test).
- Seed 18 dealer, 4 sales + CEO/admin/finance, 379 SO, 379 invoice, 85 sinyal (68 WA), stok 4 cabang — digenerasi dari data contoh mockup (`tools/seedgen`) dan idempoten.
- `web/`: Vite + React 19 + TS, `tokens.css` + `mockup.css` disalin verbatim dari mockup, font self-host, proxy `/api`.
- `Makefile` (`dev`, `check`, `test`, `lint`, `migrate`, `seed`, `sqlc`, `seedgen`, `e2e`), `.golangci.yml`, CI `check.yml`, `infra/docker-compose.yml`, `.env.example`. ADR 0006.

## Stage 01 — Metrics engine & API baca · 2026-10-05
- `internal/domain` (tipe murni + PolicySet + akar terduga) dan `internal/metrics` (semua rumus `01-glossary.md`, fungsi murni).
- Uji: 16 kasus tabel glossary, uji properti (skor 0–100, status monoton, ambang segmen), regresi seed — 18 dealer persis mockup (status, segmen, sisa limit, skor + 5 komponen, jadwal order).
- `internal/dealersvc` (muat riwayat → `metrics_current` + `dealer_metrics_daily`), job river `metrics.recompute` (10 mnt) & `metrics.snapshot` (00.30 WIB); `arc ctl recompute`, `arc ctl metrics --dealer`.
- API: `/sales`, `/dealers` (+ pencarian trigram), `/dealers/{id}` (+ orders, mix, credit, contacts, commitments, timeline, memo), `/dealers/due|drift|credit-tight`, `/orbit` (+ summary, movers), `/segmen` (+ summary, movers dari snapshot 90 hari / `metrics_prev.json`), `/kpi`, `/agenda`, `/stock/aging|critical|push|sales-by-product`, `/credit/overview|dealers|exposure|forecast`.

## Stage 02 — Frontend shell, Pusat kendali (baca), Orbit, Segmen, Dealer · 2026-10-05
- `web/src/app`: router semua rute 08-frontend, Shell (Rail, Topbar + view tabs Orbit/Segmen/Peta relasi, ⌘K, Tabbar, Dock), React Query, SseProvider (`/api/events`, heartbeat 25 dtk).
- Komponen mengikuti kelas CSS mockup (`tokens.css` + `mockup.css` verbatim): Pill, ScoreRing, Toast, Sheet, ActBtn.
- Layar: Pusat kendali (baca), Orbit (port `renderOrbit`, geometri teruji), Segmen (port `renderKuad`, skala log teruji), Dealer (header KPI, anchors, order-to-cash, share of wallet & product mix, sisa limit, memori, PIC aktif, komitmen, timeline). Layar tahap berikutnya berupa placeholder berlabel tahap.
- API: `/api/brief/today` (ringkasan 4 poin, template dari metrik), `/api/events` (Postgres LISTEN/NOTIFY → SSE), `sample_data` di `/api/health`; urutan dealer & sales mengikuti id Odoo.
- Uji: vitest (fmtRp, parser ⌘K, geometri orbit 5 kasus, skala segmen), Playwright smoke 6 rute × 1440/390 + screenshot.
