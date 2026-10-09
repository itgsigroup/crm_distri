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

## Stage 07 — MCP server, MCP sebagai orchestrator, Koneksi AI · 2026-10-05 (done-with-fakes)
- `internal/mcp`: Streamable HTTP di `/mcp` (SDK Go resmi) + `arc ctl mcp-stdio`; token `arc_…` (argon2id, tampil sekali), scope per tool, rate limit 60/mnt dan `max_cycles_per_hour`, `mcp_calls` + `audit_log` + SSE `mcp_call`.
- Tool baca (`dealer.*`, `segmen.list`, `jadwal.*`, `kredit.check`, `stok.aging`, `chat.thread` bermasking, `kpi.utama`, `cycles.recent`, `proposals.list`), analisis (`analisis.dealer|segmen|kas|stok`), orkestrasi (`orchestrator.run|reanalyze|agent.run|plan|plan.update|input.get|submit|status`); `actions.decide` → `human_only`. Resources & prompts.
- Orchestrator jalur `mcp`: migrasi `0007_mcp.sql` (`cycle_inputs`), Input per agen dimasking, submit tervalidasi (kind, provenance, dealer), Analisis menunggu ≤ 10 mnt lalu `partial`; `plan_change` dari MCP masuk Keputusan dan diterapkan setelah approve.
- API `/mcp/info`, `/mcp/clients` (buat/cabut token, CEO), `/mcp/calls`, `/policies/mcp` (`allow_send` terkunci), `/policies/llm`; CLI `arc ctl mcp-token`; `tools/mcpcheck`.
- Web: panel *MCP sebagai orchestrator* (saklar izin, log panggilan, tool), Pengaturan → Koneksi AI (jalur analisis, status, endpoint + salin, klien & token). `docs/mcp-clients.md`, ADR 0009.

## Stage 08 — Peta relasi 3D, pola relasi, PIC aktif · 2026-10-06
- Migrasi `0008_relasi.sql` (`interactions_monthly`, baseline kontak); seed riwayat interaksi 6 bulan dari mockup (`db/seed/interactions.json`); interaksi setelah `as_of` dihitung live dari WhatsApp + order.
- `views/relasi.go`: graf per periode & filter sales, pasangan terkuat, insight relasi (fungsi murni, diuji); API `/relasi`, `/relasi/insights`.
- PIC aktif dihitung ulang dari pesan kontak sebelum metrik; AI Follow-up meminta nomor admin/kasir bila hanya 1 PIC.
- Web: layar Peta relasi 3D (`three` 0.160, `NetView.ts` port `createNet`, layout di Web Worker ber-seed, fallback 2D, reduced-motion), vitest layout, e2e. ADR 0010.

## Stage 09 — Agen stok/penagihan/prospek, layar Push stok & Kredit · kas · 2026-10-06 (done-with-fakes)
- AI Stok: transfer antar cabang dan permintaan PO untuk stok kritis (keputusan → catatan Odoo, Stage 12); seed menulis sinyal `stock` seperti sinkron Odoo.
- AI Penagihan & Follow-up: `installment` untuk dealer yang minta tempo; usulan gabungan Mitra Jaya berjenis `installment`.
- AI Prospek & `internal/identify`: profil WA Business (worker, `wa.ProfileReader`), Truecaller (adapter, fake), impor CSV Getcontact, Odoo; skor; job `identify.number`; `price_list` dikirim ke thread nomor baru setelah approve. Migrasi `0009_identify.sql`.
- Prediksi kas masuk 30 hari dikalibrasi (tepat waktu, pola bayar, lewat tempo, minta tempo) → Rp 0,99 M pada seed.
- API `/chat/identify`, `/identifications/import`; `asked_tempo` di `/credit/forecast`; usulan per thread di konteks chat.
- Web: layar **Push stok** dan **Kredit · kas** dari API (port `renderAging`/`renderAR`), Chat → Nomor baru (Buat dealer tier C / Kirim harga), Pengaturan → Identifikasi nomor (impor CSV). ADR 0011.

## Stage 10 — Memori dealer, Ringkasan, kalibrasi, Tanya · 2026-10-06 (done-with-fakes)
- `internal/memory`: memo dealer sebagai kalimat bersumber (≤ 120 kata, validator menolak kalimat tanpa sumber), atribusi memo lama, penyegaran fakta saat ada sinyal baru, dirapikan LLM; ditulis di tahap Belajar. Seed menulis sinyal invoice terbuka seperti sinkron Odoo.
- Ringkasan Orchestrator ditulis siklus penuh: teks per poin dengan tautan dealer + `signal_ids`, LLM dengan validator tautan/angka; `/brief/today` membaca ringkasan siklus.
- Pelajaran kalibrasi: ≥ 3 penolakan alasan sama → pelajaran 14 hari yang menahan usulan serupa; `/calibration` memuat pelajaran.
- `internal/ask` + `POST /ask`: router pertanyaan, jawaban dari data dengan sumber.
- Web: Memori dealer (hover/klik → sumber per kalimat), Ringkasan dari siklus, Kalibrasi agen dengan pelajaran, Sheet jawaban ⌘K. Migrasi `0010_memory.sql`, ADR 0012.

## Stage 11 — Pengaturan kebijakan, auth & peran, Panduan · 2026-10-06
- `internal/policy`: JSON Schema per kebijakan + aturan terkunci (SOP-SEC-001, rilis CEO, MCP tanpa kirim, kredit tidak otonom); `Save` berversi dengan history, audit, NOTIFY; `PUT /policies/{key}`, `GET /policies/{key}/history`. Ambang yang berubah membuat siklus berikutnya menghitung ulang semua dealer.
- `internal/auth`: kata sandi argon2id, sesi JWT HttpOnly (stdlib), rate limit login; `/auth/login`, `/auth/logout`, `/users`; `arc ctl user add|passwd`; akun gudang di seed; `ARC_DEMO_PASSWORD` untuk dev.
- RBAC: menu per peran dari `/me`, keputusan per jenis + kepemilikan dealer, daftar dealer sales difilter server.
- Web: halaman login & keluar, menu/aksi per peran, Pengaturan dapat diedit (editor kebijakan + riwayat, Pengguna & peran, Matriks otonomi per sel), layar **Panduan** (isi mockup, tombol ke layar terkait). ADR 0013.

## Stage 12 — Eksekusi nyata: Odoo SO draft, outbox WA produksi, komitmen · 2026-10-06 (done-with-fakes)
- Outbox sebagai jalur eksekusi tunggal: `wa`, `odoo_so_draft`, `odoo_note`; status `pending/sent/failed/manual`, job `outbox.send` 3 percobaan, NOTIFY `outbox_failed` → toast; berhasil → proposal `executed` + audit.
- Odoo: `Source.PostNote` (`message_post` catatan internal); SO draft dengan catatan sumber; catatan keputusan di partner (kebijakan `odoo.write.notes`); nomor baru → partner baru; transfer/PO `manual`. SO draft dicatat sebagai order `ai_order_draft`, sinyal `so`, komitmen Kami "Kirim … Senin" (mengadopsi janji kirim dari chat).
- WhatsApp: jendela kirim proaktif 08–18 WIB (`WA_SEND_HOURS`), kirim pertama membuka thread, jejak "Terkirim HH.MM oleh … · dari proposal …" di Timeline.
- `internal/commitment`: reply tracking 72 jam (`chat_messages.proposal_id`, `reply_to`), janji bayar → komitmen Mereka bertanggal, jawaban ya → "Order sesuai rekomendasi"; komitmen lewat tanggal → `late` → AI Penagihan "Janji bayar terlewat" (selalu approve).
- Timeline dealer: keputusan (Disetujui/Ditolak/Otonom) dan "Balasan untuk: …". `arc ctl reanalyze` ikut mengantrekan eksekusi otonom. ADR 0014.

## Stage 13 — Hardening: keamanan, partisi & retensi, backup, observabilitas, deploy · 2026-10-06 (done-with-fakes)
- Keamanan: 2FA TOTP untuk CEO/admin (Pengaturan → Keamanan akun, kode di login, anti-replay, `arc ctl user totp-reset`), `arc ctl check-env`, CI menjalankan govulncheck + `npm audit` + uji check-env, header Caddy (HSTS, CSP, nosniff, frame DENY).
- Data: migrasi `0011_hardening` — partisi bulanan `signals` & `chat_messages` (data dipindah), tabel kunci dedupe `signal_keys`/`chat_message_keys`, job `partitions.ensure` & `retention.purge`, `arc ctl retention purge|partitions`, `arc ctl pdp export|delete`.
- Backup: `infra/backup.sh` (pg_dump terenkripsi age/OpenSSL, rotasi 30 hari), `infra/restore.sh`, `infra/restore-test.sh` + `make restore-test` + workflow mingguan `restore-test`; `arc ctl fingerprint`.
- Observabilitas: `/api/health` rinci untuk CEO/admin, `/metrics` Prometheus (histogram per rute), kartu **Status sistem**, alert WA terputus / siklus gagal 2× / antrean > 500 ke grup WhatsApp internal (outbox `wa_system`, ditolak bila bukan grup internal).
- Deploy: `infra/Dockerfile`, `infra/docker-compose.prod.yml`, `infra/Caddyfile`, `infra/systemd/`, `docs/DEPLOY.md`, `docs/RUNBOOK.md`; `arc ctl seed --policies-only` untuk produksi.
- Performa: `tools/loadtest` (+ `k6.js`, `make loadtest`), `docs/PERF.md`; timeline dealer 1,9 → 0,2 ms (join uuid + indeks parsial). ADR 0015.

## Stage 14 — Pilot cabang Semarang: perangkat pilot · 2026-10-06 (in-progress — menunggu pilot nyata)
- Kebijakan `pilot` (off / bayangan / live) + migrasi `0012_pilot`. Mode bayangan: tidak ada langkah otomatis, keputusan tetap mengkalibrasi, outbox `shadow` tidak pernah dikirim, balasan dari layar Chat ditolak, `ApproveBySystem` menolak.
- `internal/pilot`: % saran diterima & median waktu keputusan per agen, confidence mingguan, order tepat jadwal, DSO, lewat jadwal tertangkap sebelum churn, audit privasi & kirim, aturan buka otonomi ≥ 80% dua minggu (dicek server), snapshot mingguan (`pilot.snapshot`) & CSV.
- API `/pilot`, `/pilot/export.csv`, `/pilot/mode`, `/pilot/unlock`, `/sow/top`, `/sow/confirm`; `arc ctl pilot start|live|off|status|audit|snapshot|export`, `arc ctl wa import-internal`.
- Web: Pengaturan → **Pilot** (mode, KPI, per agen, audit, laporan mingguan), kartu Pilot di Pengaturan, chip **Mode bayangan** di topbar, Dealer → **Konfirmasi share of wallet** (20 dealer teratas, per kuartal).
- `docs/PILOT-REPORT.md` (checklist hari 0, aturan pilot, tabel hasil, temuan, go/no-go untuk ditandatangani Sam). ADR 0016.
- Perbaikan: `pdp delete` kini juga menghapus sinyal percakapan berdasarkan `payload.from_number`.
- Gladi pilot (`arc ctl pilot rehearse`, dev): 21 hari simulasi (14 bayangan + 7 live) — `docs/PILOT-REHEARSAL.md`. Perbaikan dari gladi: audit tidak lagi menghitung catatan Odoo atas saran yang ditolak sebagai kirim tanpa persetujuan; transfer/PO/bundle yang sudah disetujui tidak diusulkan ulang selama 7 hari (AI Stok 214 → 82 saran dalam 21 hari).

## Deploy staging · 2026-10-06
- `https://crm-distri.gsiindo.id` di VPS GSI (nginx + certbot bersama aplikasi lain): user `arc`, database `distri_arc`, API `127.0.0.1:8110`, systemd api/worker/backup, `infra/deploy.sh` (git pull → build → migrasi → restart → cek health), `infra/nginx/distri-arc.conf`. Data contoh, adapter fake, akun dengan kata sandi acak.

## Tampilan · 2026-10-06
- Tema **Terang** sebagai default (tidak lagi mengikuti mode gelap OS), dengan pilihan Terang / Gelap / Otomatis.
- Warna sidebar mejikuhibiniu: Netral, Merah, Jingga, Kuning, Hijau, Biru, Nila, Ungu — dari menu akun (semua peran) dan Pengaturan → Tampilan; disimpan per perangkat. Badge tetap terbaca di semua warna.
- Sidebar Biru memakai gradasi biru tua → nila (seperti aplikasi GSI lain) dengan penanda putih di menu aktif; sidebar bisa **diringkas** (ikon saja, tooltip nama menu, diingat per perangkat); kartu profil berisi avatar, nama, email, dan tombol keluar langsung.

## WhatsApp Baileys, banyak nomor · 2026-10-06
- Transport `baileys` (ADR 0017): bridge Node `apps/wa-bridge` (Baileys 7, sesi di Postgres, satu sesi per nomor) + transport di worker (event masuk HMAC, pemeriksaan persetujuan outbox sebelum setiap kirim). Layanan systemd `distri-arc-wa-bridge`, dibangun oleh `infra/deploy.sh`; target Docker `wa-bridge`.
- Penjaga anti-blokir: hanya membalas kontak yang pernah menghubungi, opt-out, anti-broadcast, jam tenang, 20/jam & 120/hari per nomor, pemanasan nomor baru, jeda acak, "mengetik…". Penolakan jeda menunda, penolakan final menggagalkan dengan alasan.
- Banyak nomor: migrasi `0013_wa_accounts` (percakapan milik nomor), tambah / pasangkan / lepas nomor di Pengaturan → WhatsApp dengan penghitung anti-blokir, bar nomor di Chat dengan QR langsung, "via nomor" di tiap percakapan.
- Privasi: sales hanya melihat & membuka percakapan nomornya sendiri; `/wa/pair` dibatasi CEO/admin/pemilik nomor.

## Data asli & master data · 2026-10-06
- Impor data asli (ADR 0018): kontrak kolom baku untuk tim sales, pelanggan, faktur, item faktur, stok; sumber **BigQuery** (SQL per entitas, kunci service account terenkripsi, sinkron berkala, tes query) atau **CSV** (unggah + template). Staging → transformasi idempoten → metrik dihitung ulang.
- Master data: mapping cabang, gudang → cabang, kategori → 6 kategori product mix, sales, jenis pelanggan (nilai baru tercatat sebagai "belum dipetakan"); master pelanggan (jenis, tier, limit, termin, sales) yang tidak tertimpa impor; tim sales.
- Dua jenis pelanggan: **Dealer (reseller)** dan **Freelance / System Integrator** — filter di Dealer, badge di halaman dealer, `?type=` di API board.
- Pengaturan → **Data & master**; `arc ctl wipe --confirm <db>`; setiap akun baru mendapat profil keputusan otomatis. Panduan: `docs/DATA-IMPORT.md`.

## Pemetaan BigQuery (Accurate) · 2026-10-07
- `GET /api/data/schema`: daftar dataset, tabel, kolom, dan jumlah baris project BigQuery (REST baca saja), plus saran per jenis data: tabel paling cocok (dan kandidat lain), pemetaan kolom → kontrak dari nama kolom Accurate / Indonesia (`nomor_faktur`, `sisa_tagihan`, `gudang`, …), dan SQL siap pakai.
- Pengaturan → Data & master → **Pemetaan BigQuery**: pilih tabel, cocokkan kolom (boleh ekspresi SQL), filter WHERE, pratinjau SQL, simpan semua query (CEO), tes query, jelajahi semua tabel.
- Mapping master: saran otomatis untuk kategori → 6 kategori product mix dan jenis pelanggan → reseller / SI ("saran: …", tombol **Isi saran**); tetap dikonfirmasi manusia sebelum disimpan.

## Chat WhatsApp (Baileys) · 2026-10-07
- Halaman Chat tanpa percakapan kini menampilkan **Hubungkan WhatsApp**: tambah nomor (CEO/admin) lalu tautkan, daftar nomor dan statusnya, ringkasan aturan anti-blokir; sales tanpa nomor diberi petunjuk. Chip **+ Nomor** di bar nomor.
- Menautkan dengan **kode di HP** (8 karakter, "Tautkan dengan nomor telepon saja") selain QR — `POST /wa/pair {method:"code"}`, bridge `POST /sessions {phone_code:true}`.
- Nama kontak dari HP (nama tersimpan / bisnis / push name) jadi judul percakapan nomor baru; riwayat setelah menautkan masuk sebagai sudah dibaca dan tidak memicu identifikasi profil.
- Anti-blokir: chat dibaca (centang biru) hanya tepat sebelum membalas, dengan jeda membaca 1–3 dtk, lalu "mengetik…".
- Chat **banyak nomor**: rel nomor WhatsApp di kiri (seperti akun di WhatsApp Business) — "Semua" + tiap nomor dengan avatar, status tertaut, dan jumlah belum dibaca; klik untuk melihat chat nomor itu saja, nomor yang belum tertaut langsung dibuka untuk ditautkan, **+ Nomor** untuk menambah. Kepala daftar menunjukkan nomor terpilih / jumlah nomor terhubung. Di HP rel menjadi baris yang bisa digeser.

## Master peran & akses · satu pengguna satu nomor WhatsApp · 2026-10-07
- **Peran & akses** (Pengaturan, ADR 0019): tambah/ubah peran — nama, jenis akses dasar (Admin / Finance / Sales / Gudang), menu yang dibuka, jenis saran AI yang boleh diputuskan, boleh memegang nomor WhatsApp. Peran hanya mempersempit akses dasarnya; menu ditegakkan juga di API (403), keputusan di luar peran ditolak. Hanya CEO yang mengubah; admin melihat. Migrasi `0015_roles_access`.
- **Pengguna**: peran dipilih dari master peran, cabang, dan **satu nomor WhatsApp** per pengguna (unik); daftar menunjukkan nomor dan status WA-nya. Harus selalu ada satu CEO aktif.
- **Tambah nomor di Chat dari master pengguna**: pilih pengguna → nomornya terisi dari master → Tambah & tautkan; satu pengguna satu nomor; pengguna non-admin bisa menautkan nomornya sendiri. Form yang sama di Pengaturan → WhatsApp.
- Menu **Pengguna** di sidebar (CEO/admin, menu `users` di master peran): halaman sendiri berisi master pengguna dan Peran & akses; Pengaturan menautkan ke sana. Di layar laptop pendek kartu agen di sidebar disembunyikan agar kartu profil tetap terlihat.
- Pop-up (tambah pengguna, tautkan nomor, keputusan, dll.) muncul **di tengah layar** di laptop/desktop; di HP tetap dari bawah.
- Siklus "terakhir" diurutkan menurut nomor siklus, bukan jam mulai — jam yang mundur (jam simulasi dev, koreksi NTP) tidak lagi menyembunyikan siklus baru.

## Peran custom · master cabang · WhatsApp scan lalu pilih pengguna · 2026-10-07
- **Peran & akses tanpa hardcode** (ADR 0020, migrasi `0016`): tiap peran dicentang akses per halaman (14 halaman, dikunci juga di server), cakupan data (semua / hanya miliknya), jenis keputusan, hak kebijakan (setara CEO), dan izin memegang WhatsApp. Peran bawaan bisa diubah/dihapus; selalu ada satu pemegang hak kebijakan aktif; rilis kredit di atas limit tetap butuh hak kebijakan.
- **Master cabang** (menu Master data → Cabang): tambah/ubah/nonaktifkan; ganti nama ikut ke pengguna, dealer, dan mapping data; cabang pengguna dan mapping cabang/gudang dipilih dari master.
- **Tambah pengguna** tanpa nomor WhatsApp; cabang dari master; **lihat kata sandi** (ikon mata) di form pengguna dan halaman login.
- **Chat → + Nomor**: scan QR (atau kode di HP) → nomor terbaca otomatis → pilih nama pengguna pemegangnya; nomor yang belum dipegang bisa dipilih penggunanya kemudian. `POST /wa/links`, `GET /wa/links/{id}`, `PUT /wa/numbers/{wa}/user`; bridge menyertakan nomor HP di setiap event.
- Sidebar: grup **Master data** (Pengguna, Peran & akses, Cabang).

## Push stok dari BigQuery · 2026-10-07
- Halaman Push stok sepenuhnya dari data impor (BigQuery/CSV) sesuai rumus glossary: **margin** order dihitung dari HPP (`invoice_lines.cost`, else HPP stok) sehingga **Perputaran stok** (nilai stok ÷ HPP harian) dan **Penjualan per produk** (nilai · margin) benar; **umur stok** dari `received_date` bila `age_days` kosong; **kecepatan jual** (stok kritis) dari item faktur 90 hari per SKU × cabang bila `sold_90d` kosong.
- Kontrak: `invoice_lines.cost`, `stock.received_date`; saran kolom BigQuery mengenali `hpp`/`harga_pokok`, `tanggal_masuk`/`last_purchase_date`. Tabel "bagian halaman → kolom" dan contoh SQL Accurate di `docs/DATA-IMPORT.md`.
- Pengaturan: kartu **Data asli · BigQuery (Accurate)** di atas Sumber sinyal — status kunci, **Unggah kunci JSON** langsung (Project ID terisi dari kunci, sumber otomatis jadi BigQuery), lalu "Lanjut: petakan tabel Accurate" ke Data & master. Kartu yang sama di atas halaman Data & master (tombol unggah tidak lagi tersembunyi di mode "Belum").

## Data Accurate tersambung · 2026-10-07
- BigQuery `accurate_data` terpasang (sinkron 60 menit): 37 sales, 2.841 pelanggan B2B (reseller/SI), 24.084 faktur & 52.798 item 18 bulan, 1.455 stok barang × cabang; master Cabang (Semarang/Kantor Pusat, Yogyakarta, Jakarta, Surabaya, Mataram, Soetta); 131 kategori → 6 kategori. Detail di `docs/DATA-IMPORT.md`.
- Sinkron besar: batas waktu job 30 menit; staging item faktur tanpa hapus per faktur (sebelumnya memindai tabel per faktur); baris yang tidak berubah tidak ditulis ulang (sinkron berikutnya hanya memproses perubahan); angka ilmiah BigQuery (`4.6355E7`) terbaca benar.
- Push stok sesuai glossary: kandidat tidak lagi memuat dealer **Churn** (berhenti order — bukan "lewat jadwal"); margin **per produk** dari HPP baris; daftar stok menua ringkas 12 teratas + "Tampilkan semua" + filter cabang; daftar Push di Pusat kendali hanya stok menua yang punya kandidat.
- AI Stok dengan data Accurate: harga bundle memakai harga jual terakhir bila belum ada harga tier; barang tanpa harga dilewati; maksimal 20 usulan push per siklus (nilai stok menua terbesar dulu); usulan dengan payload yang tidak bisa disimpan kini menjadi error (sebelumnya tersimpan kosong).

## Efisiensi untuk data asli (ribuan dealer) · 2026-10-07
- Server: papan dealer di-cache 60 detik (langkah berikutnya tetap segar tiap request; cache dibuang setelah perubahan lewat API); JSON dikompresi gzip; stok menua membawa 3 dealer kandidat teratas + jumlahnya (bukan ratusan); daftar Push di Pusat kendali 10 teratas; bundle AI Stok maksimal 30 dealer teratas ("30 teratas dari N yang cocok").
- Orbit: mode **Prioritas** (150 dealer omzet terbesar, 30 terbesar bernama) dan **Semua titik** (semua dealer sebagai titik kecil tanpa label); penataan label hanya untuk dealer bernama. Segmen: semua titik tetap, nama hanya 50 omzet terbesar. Daftar Dealer, daftar segmen, dan daftar Pusat kendali (jadwal order, lewat jadwal, limit tipis) dimuat bertahap ("Tampilkan N lagi").
- Filter sales menjadi dropdown bila sales lebih dari 8 (Orbit, Segmen, Peta relasi).

## MCP Claude · 2026-10-07
- **Claude menganalisis semua data** lewat MCP (ADR 0021): OAuth 2.1 untuk custom connector claude.ai / Claude Desktop / Claude Code — metadata (RFC 9728/8414), pendaftaran klien dinamis, PKCE S256 wajib, halaman persetujuan **Izinkan Claude** (`/claude/izin`, hanya pengguna dengan menu MCP Claude dan cakupan semua data; orchestrate hanya hak kebijakan), token akses 1 jam + refresh berputar 90 hari, bisa dicabut. Migrasi `0017_mcp_oauth`.
- Server MCP: nama tool ramah Claude (`dealer_list`, …), daftar dihalaman (`limit`/`offset`/`total`/`next_offset`), tool baru `data_ringkasan`, `penjualan_bulanan`, `produk_terlaris`, `piutang_ringkas`; perbaikan 403 di belakang nginx (pengaman DNS-rebinding SDK).
- Halaman **MCP Claude** (sidebar, menu `mcp` di Peran & akses): status, cara menghubungkan (claude.ai/Desktop, Claude Code, token manual), koneksi + cabut, izin & batas, daftar tool, contoh prompt analisis, log panggilan. nginx/Caddy meneruskan `/.well-known/oauth-*` dan `/oauth/`.

## Analisis terjadwal (MCP Claude) · 2026-10-07
- **Claude menganalisis sesuai jadwal cron** (ADR 0022): kartu *Analisis terjadwal* di halaman MCP Claude — jadwal (preset atau cron bebas, pratinjau "Senin–Sabtu pukul 07.00 · berikutnya …", jarak minimal 15 menit), prompt, izin tool, batas langkah, aktif/jeda, *Jalankan* sekarang, riwayat laporan (Markdown + daftar panggilan tool).
- Mesin: Claude (Messages API) memanggil tool MCP yang sama lewat sesi in-process; tiap panggilan tercatat di log MCP atas nama "Terjadwal · <nama>". Kunci Claude API diisi CEO (diperiksa, disimpan terenkripsi), pilihan model Opus 5.5/Sonnet 5.5/Haiku 4.5, anggaran harian. Tanpa kunci / lewat anggaran → laporan template berisi angka dari tool MCP.
- Worker: `analyst.tick` tiap menit + `analyst.run` (sekali per slot, tanpa retry). Migrasi `0018_mcp_schedules.sql` (jadwal awal *Ringkasan pagi* Senin–Sabtu 07.00).

## Halaman login baru · 2026-10-08
- Login satu layar tanpa scroll di atas **galaksi orbit animasi** (canvas: bintang berkelip, nebula, 7 orbit elips dengan planet berekor, inti berdenyut; diam bila "kurangi gerakan"): judul, 9 fitur sistem ringkas, dan kartu Masuk. Keterangan fitur menyusut di layar pendek; di HP hanya judul + kartu.
- Tombol mata **lihat kata sandi** lebih jelas di dalam kolom (menyala saat kata sandi terlihat).


## Nama & logo: GSI Orbit · 2026-10-08
- Nama sistem menjadi **GSI Orbit** ("CRM Distribusi · AI") di aplikasi, login, tab browser, MCP Claude, perangkat WhatsApp tertaut, 2FA, catatan Odoo, dan prompt AI.
- Logo baru (inti dengan titik yang mengorbit, biru → ungu) di sidebar, login, dan favicon.

## Pusat kendali lebih mudah dibaca · 2026-10-08
- Daftar dealer panjang di Rencana hari ini dan Ringkasan Orchestrator diringkas (5 nama + "+N dealer lainnya", bisa dibuka); poin ringkasan yang panjang dibatasi 4 baris + "Selengkapnya".
- Agenda sales: sales tersibuk dulu, 6 sales lalu "Tampilkan lagi", 3 dealer per sales, sales tanpa agenda mendesak dirangkum satu baris; baris dealer rata kiri dengan keterangan di kanan.
- Judul kartu sempit (Jadwal order, Lewat jadwal, Push stok, Limit tipis) tidak lagi terpecah; keterangan rekomendasi maksimal 2 baris. Tinggi halaman dengan data asli turun dari ±8.800 px ke ±3.700 px. Tanpa perubahan fungsi.

## Klasifikasi ulang Orbit & Segmen · 2026-10-08
- ADR 0023: faktur di hari yang sama = 1 order; siklus order minimal 7 hari; seringnya & omzet/bln dari pembelian aktual 6 bulan (bukan proyeksi); status **Prospek** untuk pelanggan yang belum pernah order (tidak digambar di Orbit/Segmen, dihitung terpisah, filter status di halaman Dealer); order sekali > 90 hari atau diam > 1 tahun = Churn; Key account bisa diberi syarat omzet minimum.

## Peta relasi 3D untuk data asli · 2026-10-08
- Peta menggambar sales dan dealer yang punya interaksi (WhatsApp + order) dalam 6 bulan, maksimal 250 dealer teraktif (sebelumnya semua 2.841 pelanggan sehingga layout tidak pernah selesai dan peta kosong); node sama untuk semua periode; HUD "250 dealer teraktif dari N".
- Order dihitung per hari order (bukan per faktur); filter sales menjadi dropdown untuk tim besar.

## Orbit mudah dibaca untuk data asli · 2026-10-08
- Titik digeser hanya **sepanjang lingkarnya sendiri** (tidak lagi menumpuk jadi kolom yang memotong lingkar); ukuran seragam kecil selama share of wallet masih estimasi; 12 nama terpenting dengan garis penunjuk; klik status di "Isi orbit" untuk menyorot satu lingkar. Arti glossary tetap (lingkar = status, sudut = siklus, ukuran = SOW, warna = sisa limit); data contoh 18 dealer memakai tata letak mockup.

## Segmen mudah dibaca · 2026-10-08
- Tampilan bawaan **Kotak**: 4 kotak besar (A–D) dengan nama sehari-hari (Pelanggan andalan, Rutin tapi kecil, Pelanggan proyek, Kecil & jarang), label "Sering/Jarang order" & "Order besar/kecil", jumlah dealer, omzet/bulan, bar % omzet, satu kalimat "Yang dilakukan", dan 3 contoh dealer. Dealer baru di baris terpisah. Klik kotak → daftar dealer di panel kanan (di HP otomatis bergulir ke daftar).
- Grafik titik tetap ada di tab **Peta titik**: huruf lebih besar, nama zona + nama sehari-hari, 20 nama dealer (dari 50), judul sumbu dengan kata biasa. Teks "Pindah kotak" ditulis sebagai kalimat (naik/turun ke Segmen X, Saran: …). Nama glossary (Segmen A–D) tidak berubah.

## Orbit: ringkasan, filter, daftar dealer · 2026-10-08
- **Ringkasan** di atas orbit: 4 kotak — Jadwal order minggu ini, Lewat jadwal, Tagih dulu (over limit / overdue, dengan total piutang), Churn — masing-masing dengan jumlah dealer dan omzet; klik kotak = filter.
- **Filter**: cari (dealer/kota/sales), status, jadwal order (≤ 7 hari / lewat jadwal), sisa limit, segmen, cabang; "Hapus filter (n)"; filter tersimpan di URL (`/orbit?jadwal=lewat`) sehingga bisa dibagikan. Klik status di "Isi orbit" kini menyaring.
- **Daftar dealer** di bawah orbit (mengikuti filter): status, jadwal order dalam kata ("lewat 9 hari", "besok"), omzet/bln, sisa limit, tombol tindakan; urutkan omzet terbesar / paling mendesak / paling lama tidak order. Definisi di `web/src/features/orbit/filters.ts` (diuji), sama dengan Pusat kendali.

## Orbit & Segmen: zoom, geser, layar penuh · 2026-10-09
- **Zoom sungguhan** di peta Orbit dan Peta titik Segmen: scroll mouse / touchpad (geser dua jari atau cubit — termasuk Safari di Mac) / cubit dua jari di HP / tombol − + / klik ganda (Shift+klik ganda = zoom out), 100%–800%, berpusat di kursor. **Seret** untuk menggeser. **Layar penuh** memakai seluruh layar (bentuk layar diikuti); di iPhone yang tidak punya Fullscreen API dipakai mode layar penuh CSS (tutup dengan tombol atau Esc).
- **Titik bertumpuk**: ukuran titik tetap sama di layar saat zoom, jadi zoom in memisahkan buletin yang bertumpuk; penyebaran anti-tumpuk dihitung ulang sesuai ukuran titik di layar (makin di-zoom, makin dekat ke posisi asli). Kontrol **Titik** (kecil/besar) terpisah dari zoom; titik terkecil = setiap titik tepat di posisi aslinya.
- Teks peta (nama lingkar, sumbu, zona) dan lingkaran GSI tetap berukuran normal saat zoom; garis tidak menebal. Klik titik tetap membuka dealer; seret tidak dianggap klik.
- `web/src/components/MapViewport.tsx` (+ `MapViewport.test.ts`: matematika zoom, letterbox, posisi asli pada titik terkecil, penyebaran mengecil saat zoom).
- **Segmen · Peta titik**: hanya area titik yang di-zoom/digeser; garis sumbu dengan panahnya dan judul sumbu tetap di tempat (juga di layar penuh), angka sumbu menyesuaikan bagian yang terlihat (mis. 2,3×–3× · 40–80 jt saat di-zoom). Titik dasar lebih kecil (ukuran tetap = share of wallet) supaya ratusan dealer tidak langsung bertumpuk.
- **Segmen · tidak ada lagi tumpukan di tepi**: dealer > 3,5×/bln dulu semua ditaruh di satu garis di tepi kanan (dan > 200 jt / < 4 jt di tepi atas/bawah). Sekarang rentang sumbu mengikuti data: 0–3,5× tetap linear di 80% lebar, di atas 3,5× dipadatkan (log) di 20% sisanya dengan angka 4×, 5×, 8×, 12×…; sumbu Rp melebar sesuai order terbesar/terkecil. Setiap dealer mendapat posisi aslinya (`scale.ts` `Domain`, diuji).

## Peta relasi 3D: filter, zoom, layar penuh · 2026-10-09
- **Filter seperti Orbit/Segmen** di atas peta: cari dealer/kota/sales, status, jadwal order, sisa limit, segmen, cabang, "Hapus filter (n)" dan "n dari N dealer". Dealer yang tidak cocok memudar beserta garisnya; nomor sales tetap menyala selama terhubung ke dealer yang cocok. Bisa digabung dengan filter nomor sales.
- **Zoom**: scroll mouse, touchpad (geser dua jari / cubit, termasuk Safari di Mac), cubit dua jari di HP, tombol − + dengan persen (50%–1000%; 100% = seluruh jaringan terlihat), ↺ atur ulang. **Geser**: Shift+seret, klik kanan+seret, atau dua jari. Seret biasa tetap memutar.
- **Layar penuh** untuk panggung 3D (cadangan CSS untuk iPhone, Esc untuk keluar); peta dirender ulang sesuai ukuran layar (dulu render berhenti karena `offsetParent` bernilai null pada elemen layar penuh).
- `useFullscreen` + `FullscreenButton` di `MapViewport.tsx` dipakai bersama Orbit, Segmen, dan Peta relasi.

## Peta relasi: tampilan Galaksi · 2026-10-09
- Pilihan **Jaringan | Galaksi** di peta relasi (diingat per browser). Di **Galaksi**: setiap nomor sales = **planet** (ukuran = interaksi), setiap dealer = **satelit** yang mengorbit sales terdekatnya (interaksi terbanyak); **orbit makin dekat = makin sering berinteraksi**, satelit dekat bergerak lebih cepat; warna = skor dealer. Dealer yang juga dekat dengan sales lain punya garis putus ke planet itu. Planet tersebar di lengan spiral di sekitar inti "GSI", sistemnya tidak pernah bertumpuk; dealer tanpa interaksi beredar di sabuk luar. Latar luar angkasa dengan bintang.
- Klik planet → kamera terbang ke sistemnya, satelitnya diberi nama (tanpa label bertabrakan), sistem lain meredup. Klik dealer dua kali → buka dealer. Filter, zoom (scroll/touchpad/cubit), geser, putar/miringkan, dan layar penuh berlaku di kedua tampilan.
- `relasi/galaxy.ts` (tata letak murni, diuji: orbit sesuai sales terdekat, orbit dalam = lebih cepat, sistem tidak bertumpuk, inti tetap kosong, deterministik), `GalaxyView.ts` (canvas, berjalan tanpa WebGL), kontrol & tooltip dipakai bersama (`controls.ts`, `tip.ts`).
- **Galaksi v2** (masukan Sam): planet tidak lagi sebaris — tersebar acak (tetap sama untuk data yang sama) di ruang 3D sekitar inti, tiap sistem dengan kemiringan orbit sendiri. **Zoom menuju kursor** (scroll/touchpad/cubit) sehingga terasa terbang menembus galaksi, sampai ±4000%; debu angkasa melintas saat dekat. **Prospek (lead yang belum pernah order)** ikut tampil: **komet** = prospek milik sales (orbit lonjong mengelilingi planet sales-nya, ekor menjauhi planet), **meteor** = prospek tanpa sales (melayang lurus menembus galaksi). Maks. 400 prospek, klik dua kali untuk membuka. Label planet tidak lagi bertumpuk.
- **Galaksi v3** (masukan Sam): komet tidak lagi mengorbit beramai-ramai — **sesekali satu komet melesat dari atas ke bawah layar** (tiap 3–7 detik) membawa satu prospek secara bergiliran, namanya tampil sekejap, klik untuk membuka. Meteor (prospek tanpa sales) dibatasi 30. **Latar bintang**: ±14.000 bintang berbagai ukuran & warna (sebagian berkelip, yang terang berkilau) dan pita Bima Sakti samar.

## Dashboard · 2026-10-09
- Card **Sorotan Orchestrator** dihapus dari Dashboard (permintaan Sam). Keputusan dan ringkasan Orchestrator tetap ada di Pusat kendali; header Dashboard masih menampilkan waktu ringkasan AI terakhir.
- **Jadwal order 7 → 14 hari** (permintaan Sam) di Orbit: kotak ringkasan "Jadwal order 2 minggu ke depan", filter "Jadwal order ≤ 14 hari (2 minggu)", warna hijau di kolom jadwal daftar dealer; metrik Dashboard ikut ("Jadwal order · 14 hari", satu definisi `DUE_DAYS` di `orbit/filters.ts`, diuji batas 14/15 hari). Pusat kendali dan agen AI di server tetap 7 hari.

## Push stok: tabel data · 2026-10-09
- Card **Push stok** menjadi tabel data selebar halaman: kolom Produk (SKU · kategori), Cabang, Qty, Nilai, Umur stok, Dealer yang cocok, aksi. **Cari** (produk, SKU, kategori, cabang, nama dealer yang cocok), **urutkan** naik/turun di setiap kolom (klik judul kolom; bawaan umur stok terlama), **paginasi** 10/25/50 baris. Chip cabang tetap ada. Stok kritis dan Penjualan per produk pindah ke bawah tabel. Logika cari/urut di `stock/table.ts` (diuji).

## Kredit · kas: Exposure vs limit sebagai tabel · 2026-10-09
- Card **Exposure vs limit** menjadi tabel data selebar halaman: Dealer (sales · cabang), Exposure, Limit, Sisa limit (merah bila minus), Pemakaian limit (bar + %). **Cari** (dealer, sales, cabang), **urutkan** naik/turun di setiap kolom (bawaan pemakaian tertinggi), **paginasi** 10/25/50; jumlah dealer over limit di kanan atas.
- Komponen bersama `components/DataTable.tsx` (`useDataTable`, `SortTh`, `TableSearch`, `TablePager`) dipakai tabel Push stok dan Exposure; perbandingan kolom diuji (`stock/table.ts`, `credit/exposure.ts`).

## Orchestrator: Resolusi konflik & Riwayat analisis sebagai tabel · 2026-10-09
- **Resolusi konflik**: tabel Agen (A ↔ B), Dealer, Konflik, Resolusi Orchestrator — cari, urutkan naik/turun tiap kolom, paginasi 10/25/50 (tetap konflik siklus penuh terakhir).
- **Riwayat analisis**: tabel Waktu (tanggal · jam), No., Jalur, Sinyal, Otonom, Keputusan, Konflik, Catatan — cari, urutkan naik/turun tiap kolom (bawaan terbaru), paginasi; kini memuat 100 siklus terakhir (sebelumnya 7 dari 20). Logika di `orchestrator/tables.ts` (diuji), memakai `DataTable` bersama.

## Dashboard: Kota Distribusi (mode game) · 2026-10-09
- Adegan animasi di atas Dashboard, digerakkan data asli: **Gudang GSI** (peti = stok menua, "!" = ada stok kritis), **Kantor sales** (jumlah chat belum dibaca), dan **toko dealer** di dua jalan (★ Key account, "!" At risk, papan silang = Churn, warna atap = sisa limit, label "order N hr lagi"). **Truk** mengantar paket ke dealer dengan jadwal order ≤ 14 hari, **kurir motor** menagih dealer over limit/overdue dan pulang membawa koin, **sales berjalan** ke dealer At risk untuk follow-up. Siang/malam mengikuti jam WIB (lampu jalan & jendela menyala malam hari).
- HUD ala game: omzet/bln, jumlah antaran, tagihan, follow-up, dan **Level** (dari % order tepat jadwal vs target). Klik toko → dealer, gudang → Push stok, kantor → Chat. Bisa disembunyikan (diingat per browser); metrik dashboard lama tetap di bawah. Gerak dimatikan bila pengguna memilih "reduce motion".
- `dashboard/town.ts` (pemilihan toko & tugas, diuji), `TownView.ts` (canvas, tanpa library baru), `DashboardTown.tsx`.

## Analisis ulang: pop-up hasil & update terakhir · 2026-10-09
- Hasil **Analisis ulang** kini tampil sebagai pop-up gaya SweetAlert (ikon beranimasi): **Analisis berhasil** (siklus, saran diperbarui, jam selesai, catatan), **Analisis selesai sebagian**, **Analisis gagal** (dengan penyebab), juga saat gagal dimulai atau Orchestrator masih berjalan. Berlaku untuk semua tombol Analisis ulang; tutup dengan OK atau Esc. Komponen tanpa library baru (`feedback.tsx` → `alert()`).
- **Pusat kendali**: baris "Update terakhir analisis: 5 Okt · 06.45 WIB (12 menit lalu) · siklus #12 · berhasil · berikutnya 07.00" di kartu Orchestrator, diperbarui tiap menit; saat berjalan menampilkan tahap yang sedang dikerjakan.

## Chat: perbaikan tautan nomor kedua + loading saat scan QR · 2026-10-09
- **Bug: nomor kedua tidak terdeteksi / datanya tidak diambil.** Penyebab: sheet tidak pernah di-unmount, sehingga "+ Nomor" yang dibuka lagi masih memegang sesi tautan nomor pertama (sudah "Terhubung") dan tidak meminta QR baru; "Tautkan ulang" nomor lain juga tidak mengirim permintaan pairing. Kini setiap pembukaan sheet adalah komponen baru (`feedback.tsx`, diuji). Pengaman tambahan: nomor akun pesan tidak lagi diambil dari digit acak di id sesi `link-…` (`baileys.go`, diuji).
- **Loading saat scan QR**: wa-bridge memberi tanda saat QR dipindai/kode diketik (`isNewLogin` → awalan `linking:`), UI menampilkan spinner "QR berhasil dipindai — menautkan perangkat dan mengambil data chat…" sampai nomor terhubung; juga spinner saat menyiapkan QR/kode. Tanpa migrasi (pola yang sama dengan awalan `code:`).

## Chat: nama & detail per nomor WhatsApp · 2026-10-09
- Nomor di Chat kini tampil dengan **nama pemegangnya** (sebelumnya nomor yang ditautkan lewat "+ Nomor" lalu diberikan ke sales tetap tampil sebagai "+62 899-••••-…", karena nama pengguna tidak dipakai). Rail menampilkan nama depan (nomor tanpa pemegang: "…0002" dan avatar "?"), header daftar chat: nama · peran · cabang · nomor.
- **Detail nomor** (ikon di samping nama, atau tombol Detail di daftar nomor): pemegang & peran, cabang, email, nomor lengkap (hanya untuk admin dan pemegangnya), status, tertaut sejak, terakhir aktif, chat terakhir, jumlah percakapan & belum dibaca, kirim hari ini vs batas & masa pemanasan, jenis perangkat; aksi Ganti pemegang / Tautkan ulang.
- Perbaikan: hitungan kirim per nomor (hari ini x/y, pemanasan) tidak pernah muncul untuk nomor yang ditautkan dari Chat — bridge menghitung per id sesi (`link-…`), server mencari per nomor. `ListWANumbers` kini membawa peran, cabang, dan statistik chat (sqlc diregenerasi).
- **Pemegang nomor tidak tersimpan diam-diam**: memilih sales yang sudah memegang nomor lain ditolak server (satu pengguna satu nomor) dan hanya tampil sebagai toast sekejap, sehingga nomor baru tetap tanpa nama — sales itu bahkan tidak muncul di pilihan. Kini semua pengguna yang boleh memegang WhatsApp tampil (ditandai "memegang …1234"), memilih yang sudah memegang nomor memunculkan peringatan dan tombol **Pindahkan ke nomor ini** (nomor lama tetap tersambung tanpa pemegang; `move: true` di API), dan setiap kegagalan simpan tampil permanen di form + pop-up.
- **Nomor bisa dipegang sales tanpa akun.** Sales lapangan dari master Sales (mis. impor Odoo) yang belum punya login tidak pernah muncul di pilihan pemegang, sehingga nomor yang sudah discan tetap tanpa nama. Kini form pemegang punya grup **Sales (tanpa akun)**; memilihnya mengisi nama nomor, menautkan percakapan ke sales itu, dan menjadikannya nomor utama sales bila belum ada (`PUT /wa/numbers/{wa}/user` dengan `sales_id`, hanya pemegang menu Pengguna; sales yang punya akun tetap lewat pengguna). `/sales` kini menyertakan `id`.
