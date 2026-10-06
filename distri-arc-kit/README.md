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

