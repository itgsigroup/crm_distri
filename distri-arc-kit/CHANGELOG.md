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

## Stage 03 — WhatsApp ingest & Chat · 2026-10-05 (done-with-fakes)
- `internal/wa`: `Transport` (fake, whatsmeow linked device + backfill, Cloud API), ingest → `chat_threads`/`chat_messages`/`signals` idempoten, filter privasi, NOTIFY `chat_message`/`wa_status`.
- `internal/outbox`: kirim hanya baris outbox untuk proposal disetujui; batas harian per nomor, jeda acak, balasan manusia dengan jeda mengetik (ADR 0007).
- Migrasi `0003_whatsapp.sql` (wa_numbers, skema whatsmeow, thread unik per nomor sales, status pesan); seed nomor internal, grup gudang, 4 nomor sales, 6 thread mockup, identifikasi nomor baru.
- API: `/chat/threads` (+ detail, read, context, messages), `/internal-numbers`, `/wa/groups`, `/wa/status`, `/wa/pair`, `/wa/cloud/webhook`, `/policies`. `arc ctl wa inject|numbers`.
- Web: Chat 3 panel (tab, anotasi agen, konteks dealer/grup/nomor baru, balas sebagai sales), Pengaturan (kebijakan dari DB, sumber sinyal, panel WhatsApp: QR, nomor internal, grup).

## Stage 04 — Odoo sync read-only & order-to-cash · 2026-10-05 (done-with-fakes)
- `internal/odoo`: klien JSON-RPC (API key, `search_read` berhalaman) dan `Fake` (domain =, !=, >, <, in) dari `db/seed/odoo/*.json` yang diekspor `tools/seedgen` dengan id sama dengan seed.
- Syncer: partner → dealer & kontak (tier dari pricelist, termin, cabang dari company, sales dari user), produk + harga tier (`products`), SO + baris (kategori via `category_map`), picking/invoice/pembayaran → fase Order → Siap → Kirim → Invoice → Bayar, quant → stok; sinyal `so|invoice|payment|stock` dedupe `model:id:write_date`; kursor per model (`odoo_sync_state`).
- Job river `odoo.sync` (10 mnt) + recompute dealer tersentuh; `arc ctl odoo sync [--full]`, `arc ctl odoo test`; `CreateSODraft` dengan catatan sumber (ditolak bila `ODOO_WRITE=false`).
- API `/connections`, `/connections/odoo/test|sync|categories`; Pengaturan → Odoo (status sinkron, uji, sinkron penuh, pemetaan kategori ke 6 KAT).

## Stage 05 — LLM provider, agen v1, proposal & keputusan · 2026-10-05 (done-with-fakes)
- `internal/llm`: `Provider` (Anthropic beta Messages: structured output, effort, server-side fallback; OpenAI cadangan; `Fake`), `Router` (masking PII → unmask, validator, fallback template, log `llm_calls` provider/model/token/biaya Rp/hash/cycle), prompt di `internal/llm/prompts/`.
- `internal/agents`: AI Order (ekstraksi intent & item WA → `so_draft`/`credit_hold`, `price_counter` dengan floor margin, `return`), AI Follow-up (H-1, 2–7 hari, lewat jadwal dengan akar terduga), AI Kredit (`credit_release` SOP-SEC-001 + DP 50%, `credit_limit`). Semua angka dari `internal/metrics`; provenance wajib di domain.
- Migrasi `0005_proposals.sql` (ringkasan, tombol, opsi, pills, antrean, payload, `dedupe_key` unik untuk proposal terbuka).
- `internal/proposals`: runner (kedaluwarsa harian, dedupe, supresi kalibrasi, otonom hanya SO draft lengkap + credit hold) dan `Decide` (RBAC rilis CEO / limit CEO-finance, tolak wajib alasan → kalibrasi 14 hari, approve/edit → outbox WA dari nomor sales pemilik atau catatan/SO draft Odoo, audit + NOTIFY).
- API `/proposals` (+ `?queue=1&today=1`), `/proposals/{id}` (+ sinyal sumber), `POST /proposals/{id}/decide`, `/calibration` (confidence per agen 30 hari + kejadian terbaru), `/dealers/{id}/next`; `next` di daftar dealer, anotasi proposal di chat. `arc ctl agents run [--all|--agent|--dealer|--if-empty]`; `make dev` mengisi proposal awal.
- Web: antrian Keputusan (port `renderQueue`), ActionSheet (port `openSheet`: sumber, kenapa sekarang, disiapkan, setelah disetujui, edit draft, tolak dengan alasan), tombol aksi di Jadwal order/Lewat jadwal/Limit tipis/Dealer/Chat, Kalibrasi agen di Pengaturan. Perbaikan: `FeedbackProvider` di dalam router (sheet dapat bernavigasi).

## Stage 06 — Orchestrator 6 tahap, konflik, otonomi, rencana, layar Orchestrator · 2026-10-05 (done-with-fakes)
- `internal/orchestrator`: `Run(Scope, Trigger)` → Ingest, Analisis (errgroup ≤ 4), Sintesis & konflik, Keputusan, Eksekusi, Belajar; `cycle_stages` + NOTIFY per transisi; satu siklus aktif (indeks `cycles_one_active` → `409`, advisory lock per skema); siklus ber-scope memakai proposal hari ini sebagai jangkar aturan.
- Aturan murni teruji: `suppression`, `followup_gap`, `dedupe` (covers, satu tawaran per dealer, sama jenis), `credit_over_stock`, `collect_before_followup`, `margin_floor`, `one_owner`; `Evaluate` otonomi (matriks + `autonomy.guard`); `plan.Build` (≤ 10 langkah, jam, status).
- Agen v1 baru: AI Stok (bundle di atas floor margin), AI Penagihan (H-3 ramah, tegas, cicilan), AI Prospek (nomor baru → dealer tier C). Keputusan bundle → satu draft WA per dealer; dealer baru → catatan Odoo (Stage 12).
- Migrasi `0006_orchestrator.sql`; job `cycle.run` (tiap jam 06–20 WIB + dari `POST /cycles`); `arc ctl reanalyze --scope …`, `arc ctl cycle status`; `make dev` menjalankan siklus pertama.
- API: `/cycles/latest`, `/cycles`, `POST /cycles` (202/409), `/cycles/{id}`, `/conflicts`, `/agents`, `/agents/{name}/run`, `/plan/today`, `POST /plan/{id}/run` (*Jalankan sekarang*, keputusan manusia), `/policies/autonomy` (GET/PUT CEO), `/brief/today` dari siklus terakhir.
- Web: kartu Orchestrator & pill topbar hidup, Rencana hari ini, layar Orchestrator (pipeline, resolusi konflik, riwayat, agen, matriks; panel MCP menunggu Stage 07), Dock dengan scope layar, Analisis ulang di Orbit/Segmen/Dealer/⌘K, reducer `useCycle` (vitest), e2e Analisis ulang. ADR 0008.

