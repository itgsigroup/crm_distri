# Distri ARC Orbit — Build Kit untuk Claude Code

CRM AI-native untuk distribusi B2B GSI. Kit ini berisi **desain lengkap** (`docs/design/`), **keputusan arsitektur** (`docs/adr/`), **15 prompt tahap** (`docs/stages/`), protokol eksekusi (`CLAUDE.md`, `.claude/commands/stage.md`), dan **mockup UI yang disetujui** (`reference/`). Stack: **Go 1.23 · React 19 · PostgreSQL 16**.

## Cara pakai (Sam)
1. Buat repo kosong (mis. `distri-arc`), salin seluruh isi kit ini ke dalamnya, `git init && git add -A && git commit -m "chore: build kit"`.
2. Pasang Claude Code, buka folder itu: `claude`.
3. Ketik `/stage next` (atau `lanjut`). Claude Code membaca `.arc/progress.json`, mengerjakan tahap berikutnya sampai acceptance criteria lolos, lalu melapor ≤ 10 baris.
4. Ulangi `/stage next` sampai Stage 14. `/stage status` untuk melihat posisi. `/stage plan` untuk melihat rencana tanpa eksekusi.
5. Bila Claude Code bertanya (hanya untuk keputusan yang tidak bisa dibatalkan atau kredensial), jawab singkat; jawaban dicatat di `docs/OPEN-QUESTIONS.md`.

Prasyarat mesin: Go 1.23+, Node 20+, Docker (Postgres), `make`. Kredensial (Odoo, WhatsApp, Anthropic/OpenAI) **tidak wajib** untuk tahap 00–07 — semua integrasi punya implementasi fake yang realistis; tahap ditandai `done-with-fakes` dan diisi kredensial belakangan.

## Peta tahap
| # | Tahap | Hasil yang bisa dicoba |
|---|---|---|
| 00 | Fondasi | `make dev` → Postgres + API + worker + web kosong, seed 18 dealer |
| 01 | Metrics engine | `/api/orbit` dengan status/segmen/skor persis mockup |
| 02 | Frontend shell | Pusat kendali (baca), Orbit, Segmen, Dealer |
| 03 | WhatsApp | Chat 3 panel, pesan masuk real-time, balas = keputusan |
| 04 | Odoo sync | SO/invoice/bayar/stok dari Odoo, order-to-cash |
| 05 | Agen v1 | Proposal + ActionSheet: setujui/edit/tolak |
| 06 | Orchestrator | Siklus per jam, konflik, Rencana hari ini, layar Orchestrator |
| 07 | MCP | Claude Desktop/ChatGPT membaca, menganalisis, mengorkestrasi |
| 08 | Peta relasi 3D | Graph sales ↔ dealer |
| 09 | Agen v2 | Push stok, Kredit · kas, nomor baru |
| 10 | Memori & kalibrasi | Memo berprovenance, pelajaran, ⌘K |
| 11 | Pengaturan | Kebijakan, peran, Panduan |
| 12 | Eksekusi nyata | SO draft Odoo, WA produksi, komitmen |
| 13 | Hardening | Keamanan, backup, deploy, runbook |
| 14 | Pilot | Cabang Semarang, mode bayangan, laporan |

## Prinsip yang dijaga semua tahap
AI mengusulkan · Orchestrator mengatur · manusia memutuskan. Angka dihitung di Go, bukan oleh LLM. Setiap klaim berprovenance. Odoo source of truth. Kebijakan sebagai data. Tidak ada kirim ke dealer tanpa persetujuan — dari UI, jadwal, maupun MCP.

## Menjalankan (Stage 00+)
1. Prasyarat: Go 1.26+, Node 20+, PostgreSQL 16+ (native, atau Docker: `make db-up`), `sqlc` & `golangci-lint` untuk `make sqlc`/`make check`.
2. `cp .env.example .env` lalu sesuaikan `DATABASE_URL` / `DATABASE_URL_TEST` (buat dua database kosong).
3. `make setup` (npm ci untuk `web/`).
4. `make dev` → migrasi, seed 18 dealer (bila kosong), API `:8080`, worker, web `http://localhost:5173`.
5. `make check` sebelum commit. Data contoh dibangun ulang dengan `make seedgen` (dari mockup) lalu `make reset`.

Catatan lokal: proyek ini berada di `distri-arc-kit/` di repo `crm_distri` (branch `distri-arc-orbit`); lihat ADR 0006.

### Tahap 01–02: yang bisa dicoba
- `go run ./cmd/arc ctl metrics --dealer mitra` → siklus order, status, segmen, sisa limit, skor + 5 komponen.
- `make dev` lalu buka `http://localhost:5173`: Pusat kendali, Orbit, Segmen, Dealer (data dari API).
- `cd web && npx playwright test` (dengan `make dev` berjalan) → smoke 6 rute + screenshot di `web/e2e/__screenshots__`.

### Tahap 05: agen & keputusan
- `bin/arc ctl agents run --all` (atau otomatis saat `make dev`) → proposal AI Order / Follow-up / Kredit dari seed; `--agent "AI Kredit"`, `--dealer graha`.
- Pusat kendali → Keputusan / Jadwal order: buka tombol aksi → ActionSheet → *Setujui & jalankan* (WA terkirim lewat `wa.Fake`, terlihat di Chat), *Edit dulu*, atau *Tolak* dengan alasan (tampil di Pengaturan → Kalibrasi agen).
- LLM nyata: `LLM_PROVIDER=anthropic` + `ANTHROPIC_API_KEY`; biaya per panggilan di tabel `llm_calls`.

### Tahap 06: Orchestrator
- `bin/arc ctl reanalyze --scope all` (otomatis saat `make dev`) → siklus 6 tahap, konflik, Rencana hari ini; `--scope screen:orbit`, `dealer:mitra`, `agent:AI Kredit`. `bin/arc ctl cycle status` → riwayat.
- Worker menjalankan siklus tiap jam 06.00–20.00 WIB; tombol *Analisis ulang* (Pusat kendali, Orbit, Segmen, Dealer, Dock, ⌘K "analisis ulang …") memicu siklus ber-scope; siklus kedua saat berjalan → "Orchestrator sedang berjalan".
- Langkah otonom yang mengirim ke dealer menunggu *Jalankan sekarang* (ADR 0008); kebijakan `autonomy.guard` lewat `PUT /api/policies/autonomy` (CEO).

### Tahap 07: MCP
- `bin/arc ctl mcp-token --name "Claude Desktop Sam" --scopes read,analyze,orchestrate` (atau Pengaturan → Koneksi AI → Buat token) lalu ikuti `docs/mcp-clients.md`.
- Uji cepat: `go run ./tools/mcpcheck -token arc_… jadwal.due '{"sales":"Dewi"}'` dan `… orchestrator.reanalyze '{"scope":"dealer:mitra"}'` → siklus `MCP` di Riwayat analisis + log panggilan di layar Orchestrator.

### Tahap 08: Peta relasi
- Orbit → *Peta relasi 3D* (`/orbit/relasi`): periode 30–180 hari, filter sales, pasangan terkuat, pola relasi; klik dua kali dealer → halaman dealer. API: `/api/relasi?period=90&sales=dewi`.

### Tahap 09: Push stok, Kredit · kas, nomor baru
- `/stok` dan `/kredit`: KPI, push stok (bundle), stok kritis (Usulkan transfer / Ajukan PO), sisa limit, exposure, prediksi kas — semua dari API.
- Chat → Nomor baru: identifikasi + *Buat dealer tier C* / *Kirim harga*. Impor Getcontact: Pengaturan → Identifikasi nomor, atau `curl -X POST --data-binary @getcontact.csv localhost:8080/api/identifications/import`.

### Tahap 10: memori, ringkasan, kalibrasi, Tanya
- Dealer → Memori dealer: arahkan kursor ke kalimat untuk melihat sumbernya, klik untuk membuka sumber.
- ⌘K: "dealer mana yang berisiko", "stok apa yang harus didorong", "berapa prediksi kas masuk", "siapa yang order minggu ini".
- Pengaturan → Kalibrasi agen: tiga penolakan dengan alasan sama menjadi satu pelajaran (berlaku 14 hari).

### Tahap 11: login, peran, kebijakan
- Isi `ARC_DEMO_PASSWORD` di `.env` (dev) lalu `make reset`: akun contoh (sam@, admin@, finance@, andi@/dewi@/rizky@/fajar@, gudang@ `gsi.co.id`) memakai kata sandi itu. Produksi: `bin/arc ctl user add --email … --role ceo --password …`.
- Pengaturan (CEO): ubah ambang, limit, floor margin, follow-up, matriks otonomi; setiap perubahan berversi dan berlaku di siklus berikutnya.

### Tahap 12: eksekusi nyata
- Dev: `WA_SEND_HOURS=off` (jam contoh 06.45 di luar jendela 08–18) dan `ODOO_WRITE=true` dengan `ODOO_MODE=fake`. `make reset` lalu `bin/arc ctl reanalyze --scope all`: SO draft otonom Toko Sinar dikirim worker ke Odoo fake → halaman Dealer Sinar menampilkan rantai WA → Otonom → SO draft #900001 dan komitmen Kami.
- Setujui follow-up, lalu simulasikan balasan dealer: `bin/arc ctl wa inject --from 6281900301301 --in 1h --text "INV/0964 saya transfer hari Kamis ya"` (Pak Bayu, Prima) → komitmen Mereka bertanggal dan Timeline "Balasan untuk: …".
- Produksi: `ODOO_MODE=rpc` + `ODOO_WRITE=true` hanya setelah diuji di instance Odoo uji; status tiap eksekusi ada di tabel `outbox` (gagal → toast + coba ulang).

### Tahap 13: hardening & operasi
- Pengaturan → **Status sistem** (CEO/admin) dan **Keamanan akun** (2FA). `curl localhost:8080/metrics` untuk Prometheus.
- `bin/arc ctl check-env` (gagal bila konfigurasi produksi salah) · `bin/arc ctl retention purge` · `bin/arc ctl pdp export --dealer sinar` · `bin/arc ctl pdp delete --contact 62… --yes`.
- `make restore-test` (backup terenkripsi → restore → metrik identik; butuh `PG_BIN` sesuai versi server dan OpenSSL ≥ 1.1.1 di `OPENSSL`) · `make loadtest` (50k sinyal, p95, EXPLAIN; lalu `make reset`).
- Produksi: `docs/DEPLOY.md` (Docker Compose atau systemd), insiden: `docs/RUNBOOK.md`, performa: `docs/PERF.md`.

### Tahap 14: pilot cabang
- Checklist & laporan: `docs/PILOT-REPORT.md`. Mulai: Pengaturan → Pilot → *Bayangan* atau `bin/arc ctl pilot start --branch Semarang`.
- Selama pilot: `bin/arc ctl pilot status` (per agen + KPI), `bin/arc ctl pilot audit` (exit 1 bila ada pelanggaran), `bin/arc ctl pilot export --week 2026-10-05 --out minggu.csv`.
- Sales: Dealer → **Konfirmasi share of wallet** (20 dealer teratas per kuartal). Nomor internal: `bin/arc ctl wa import-internal --csv internal.csv`.
