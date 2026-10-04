# Changelog

Diisi per tahap oleh Claude Code (format: `## Stage NN — nama (tanggal)` lalu poin perubahan).

## Stage 13 — Hardening & go-live (2026-10-04)
- Suite acceptance Go per tahap (`apps/api/internal/app/stage*_test.go`, DB `arc_test`), test bridge, vitest, Playwright — semua hijau.
- Production menolak start dengan rahasia default, demo clock, atau URL non-https (`config.Validate`).
- Watchdog sesi WhatsApp (putus > 10 menit → email CEO, sekali per gangguan), job `bridge_watch`; alert job gagal 2× dan biaya AI harian.
- `GET /api/metrics` (CEO): job, LLM 24 jam, antrean Action, sesi WA, health bridge.
- `scripts/backup.sh` / `restore.sh` (pg_dump custom, termasuk skema `wa_bridge`), `make backup/restore`.
- `make perf` (`apps/api/cmd/arcperf`): 500 akun, 10.136 interaksi → semua endpoint utama p95 < 300 ms (terburuk `/api/chat/threads` 112 ms).
- Playwright: smoke semua layar, 400 px tanpa scroll horizontal, angka mockup, scoping cabang, action sheet, dan e2e WA masuk → prospek → lead → balasan → approve → terkirim → Kas.
- Docs: runbook, go-live checklist, pelatihan sales 1 halaman, uji manual WhatsApp, koneksi Claude/ChatGPT.

## Stage 12 — Growth (2026-10-04)
- Flywheel (referral, ekspansi, time-to-delight) dengan verdict yang berbalik bila data dibalik; sumber & funnel 7 tahap.
- Tender radar (skor 92/84/71 + alasan), renewal/garansi (Amarta → `offer_maintenance`), coaching tim (Fajar "0 lead, 102 pesan").

## Stage 11 — Cash engine (2026-10-04)
- Lead-to-cash per project vs benchmark; Action `create_invoice` (Panti Rapih), pengingat ramah (Semen Mitra), `ask_spm_documents` (Magelang), `schedule_bast` (Cakra).
- Prediksi kas 30 hari tertimbang pola bayar (Rp 2,4 M) + what-if invoice (→ Rp 2,65 M), aging, ekspor CSV.

## Stage 10 — Odoo write-back (2026-10-04)
- Writer whitelist (probability, mail.activity, message_post); tidak pernah `stage_id`/`unlink`; catatan sumber di setiap tulis.
- Konflik `write_date` → tidak menulis + Signal `sync_conflict`.
- Komitmen `kami` terbuka → `mail.activity` di lead Odoo (dengan penanda "via ARC" + kutipan bukti); terpenuhi → `action_done`; job `odoo_activities` tiap jam, idempoten (migrasi 0002).

## Stage 09 — Gmail & Calendar (2026-10-04)
- Parser .eml/Gmail API, pembersih kutipan, penautan ke akun/orang; 12 email → 9 tertaut, 2 unresolved, 1 diabaikan; idempoten.
- Kalender → meeting prep H-1 (3 Action); draf Gmail untuk Action email.

## Stage 08 — Odoo read-only (2026-10-04)
- Klien XML-RPC stdlib (read-only), FakeOdoo; sync idempoten berbasis `write_date`; penautan 6 otomatis + 2 usulan (`link_to_odoo`) + `create_in_odoo`; L2C 6 kasus.

## Stage 07 — Brief & approval (2026-10-04)
- Brief pagi 06.45 / sore 16.00 (4 poin, tiap poin ber-evidence), notifier (SMTP/Basecamp/Fake).
- Approve/edit/opsi/tolak (alasan tetap)/tunda; suppress 14 hari; LearnedRule setelah 3× "Tidak sesuai kebijakan"; kalibrasi.

## Stage 06 — MCP, AI API, Ask (2026-10-04)
- `arc.accounts.brief` / `GET /api/v1/accounts/{id}/brief` menyertakan daftar evidence.
- MCP Streamable HTTP (2025-06-18) + OAuth 2.1 (PKCE S256, dynamic client registration, resource metadata); `arc.actions.decide` ditolak untuk mesin; toggle grup tools.
- REST `/api/v1` + OpenAPI 3.1, API key (scope, rate limit, last used), webhook keluar HMAC; Ask dengan evidence cards dan saran per layar.

## Stage 05 — Pipeline intelligence (2026-10-04)
- Health & flag per opportunity, forecast commit/best/pipeline (Rp 4,3/5,9/11,9 M) + what-if (BSD → 3,4), weighted ARC.
- NBA table-driven (`rules/nba.json`), Prospek inbound → lead dengan pertanyaan terlampir, Action sheet global.

## Stage 04 — Web v1 (2026-10-04)
- React + Vite dengan CSS mockup apa adanya: Hari ini, Chat (anotasi), Relasi + peta stakeholder, Peta 3D (Three.js), Penjualan, Prospek, Kas, Ask, Pengaturan (semua tab). Semua data dari API.
- Perbaikan kecil di luar mockup hanya untuk 400 px tanpa scroll horizontal.

## Stage 03 — Reasoning core (2026-10-04)
- Router LLM (light/heavy/interactive), masking PII sebelum provider eksternal (termasuk format "+62 812-…"), pencatatan token/biaya, FakeProvider deterministik.
- Capture komitmen/sinyal/tugas dengan evidence & confidence (ekstraksi ulang tidak menggandakan); identitas inbound; research; memori akun; hygiene; eval → `docs/eval/` (FakeProvider: 18 kasus, precision 100%, recall 100%; laporan mencantumkan FP & FN).

## Stage 02 — WhatsApp bridge (2026-10-04)
- `apps/wa-bridge` (Go + whatsmeow): multi-sesi QR, riwayat N hari, grup + anggota, profil, kirim hanya untuk Action approved (dicek ulang ke API), ≤ 20/jam/sesi, jeda 2–6 dtk, antrean file saat API mati, `/health`.
- API: webhook HMAC idempoten per wamid, klasifikasi grup, privasi (internal 1:1 tidak disimpan, grup non-opt-in hanya counter — counter idempoten saat replay), registry nomor internal, parser ekspor chat (dedupe terhadap event live), Cloud API adapter.

## Stage 01 — Domain model (2026-10-04)
- Skema PostgreSQL lengkap dengan `source_system/source_id`, migrasi SQL tertanam, audit log append-only; seed fixture mockup idempoten; health v1 (RSUD 43 ± 3).

## Stage 00 — Bootstrap (2026-10-04)
- Struktur repo, ADR 0003 (Go + React + PostgreSQL menggantikan stack ADR 0001/0002 atas permintaan pemilik), Makefile, `.env.example`, compose + Caddy, konfigurasi gitleaks/pre-commit.
