# Distri ARC Orbit — Instruksi untuk Claude Code

Anda membangun **Distri ARC Orbit**: CRM AI-native untuk bisnis **distribusi B2B** PT Gosyen Solusi Indonesia (GSI) — penjualan ke dealer/toko/installer dengan termin kredit. Produk ini bukan pipeline/kanban. Intinya: **Orchestrator** yang mengatur enam agen AI untuk menjaga dealer tetap "di orbit" (order rutin, limit sehat, product mix melebar), dan **manusia yang memutuskan**.

Proyek dibangun **bertahap** (Stage 00–14). Pemilik proyek (Sam, CEO GSI) hanya memberi perintah singkat: `/stage next`, `lanjut`, `/stage status`. Andalah yang harus tahu tahap mana yang berjalan, apa yang selesai, dan apa syarat selesainya.

## 0. Baca ini dulu, setiap sesi
1. `.arc/progress.json` — status tiap tahap. Sumber kebenaran posisi proyek.
2. `docs/design/00-overview.md` dan `01-glossary.md` — **selalu**. File design lain sesuai tahap.
3. `docs/stages/NN-*.md` — prompt tahap yang akan dikerjakan.
4. `docs/adr/*.md` — keputusan arsitektur. Jangan dibatalkan diam-diam; bila perlu berubah, tulis ADR baru yang menggantikan.
5. `reference/distri-arc-orbit-v2-mockup.html` — mockup UI yang sudah disetujui (buka di browser). Untuk tahap frontend ini adalah **spesifikasi visual dan interaksi**: token warna, tipografi, komponen, teks UI, perilaku. `reference/panduan-orbit.html` — penjelasan istilah dan rumus untuk tim.

## 1. Protokol eksekusi bertahap
Saat menerima `/stage next`, `lanjut`, atau perintah serupa:
1. Baca `.arc/progress.json`. Tahap berikutnya = tahap pertama dengan `status != "done"`. Jangan melompat.
2. Baca prompt tahap di `docs/stages/`, lalu design doc yang dirujuknya.
3. Tulis rencana singkat (≤ 12 baris): file yang disentuh, urutan, risiko. Lalu kerjakan — jangan menunggu persetujuan kecuali ada keputusan yang **tidak bisa dibatalkan** dan bisa jatuh dua arah (§3).
4. Setiap tahap harus lolos **acceptance criteria** di promptnya: `make check` hijau (test + lint + typecheck), `make dev` jalan. Jalankan sendiri; jangan mengklaim.
5. Bila terhalang kredensial/API eksternal (Odoo, WhatsApp, LLM): buat adapter dengan **interface + implementasi fake** yang realistis dan deterministik, tandai `status: "done-with-fakes"`, catat kebutuhan di `docs/OPEN-QUESTIONS.md`. Tahap tidak boleh macet karena kunci API.
6. Setelah lolos: perbarui `.arc/progress.json` (`status`, `finished_at`, `summary`, `notes`), tambah entri `CHANGELOG.md`, commit `feat(stage-NN): <ringkasan>`.
7. Laporkan ke Sam ≤ 10 baris: apa yang jadi, cara mencoba (perintah), yang belum, pertanyaan terbuka.

Perintah lain: `/stage status` (tabel + saran), `/stage N` (hanya bila semua < N selesai), `/stage redo N` (perbaiki dari acceptance criteria, bukan tulis ulang), `/stage plan` (rencana tanpa eksekusi).

## 2. Prinsip produk yang tidak boleh dilanggar
- **AI mengusulkan, Orchestrator mengatur, manusia memutuskan.** Tidak ada pesan, harga, rilis kredit, atau perubahan limit yang sampai ke dealer tanpa keputusan manusia yang tercatat (`proposals.decided_by`). Endpoint keputusan hanya untuk pengguna manusia; klien MCP **tidak pernah** bisa mengirim.
- **Orchestrator-first.** Agen tidak dipanggil langsung oleh UI. Semua analisis — siklus per jam, tombol *Analisis ulang*, panggilan MCP — masuk lewat `orchestrator.Run(ctx, Scope, Trigger)` dan menghasilkan `cycle` yang tercatat lengkap (6 tahap, konflik, keputusan).
- **Odoo adalah source of truth** untuk dealer master, SO, invoice, pembayaran, stok. Distri ARC menyimpan salinan bertanda `source_system/source_id/source_write_date` dan **tidak pernah** mengubah data Odoo kecuali membuat SO draft (Stage 04+) dengan catatan sumber. Sebelum Odoo tersambung, seed dari `db/seed/` (18 dealer contoh dari mockup).
- **Setiap klaim AI membawa provenance**: `signal_ids`, waktu, `confidence`. Proposal tanpa provenance ditolak di layer domain, bukan di UI.
- **Semua angka Orbit dihitung dari rumus di `01-glossary.md`**, di Go (package `metrics`), bukan oleh LLM. LLM hanya menulis alasan, draft pesan, dan memilih di antara opsi yang sudah dihitung.
- **Idempoten**: menjalankan ulang job apa pun tidak menggandakan data (kunci unik: wa message id, odoo id + write_date, cycle id + agent).
- **Privasi** (`09-policies-security.md`): nomor internal dikenali dan DM antar karyawan tidak dibaca; grup internal hanya untuk stok/jadwal/tugas; identifikasi nomor hanya untuk nomor inbound; chat disimpan 90 hari; PII dimasking sebelum ke provider LLM eksternal.
- **Bahasa**: UI dan teks untuk pengguna dalam Bahasa Indonesia dengan istilah bisnis umum (Key account, share of wallet, DSO, follow-up). Istilah produk mengikuti `01-glossary.md` persis. Kode, komentar, nama identifier, commit: Inggris.

## 3. Kapan bertanya, kapan memutuskan sendiri
Putuskan sendiri dan catat di ADR: pilihan library (dalam batas §4), struktur package, skema tabel, nama endpoint, format prompt LLM.
Berhenti dan tanya (satu pertanyaan, opsi jelas) hanya untuk: menulis/menghapus data Odoo produksi, mengganti stack di ADR, membeli layanan berbayar, mengirim apa pun ke dealer sungguhan, dan keputusan lain yang tidak bisa dibatalkan.

## 4. Stack (ADR 0001–0005)
- **Backend**: Go 1.23+. `chi` router, `pgx/v5` + `sqlc` untuk query bertipe, migrasi `goose` (SQL), job & scheduler `river` (antrean di Postgres, tanpa Redis), `slog` JSON logging, `otel` opsional. LLM lewat `internal/llm` (interface `Provider`; implementasi Anthropic dan OpenAI; `Fake` untuk test). MCP server di `internal/mcp` (Streamable HTTP, SDK Go resmi `modelcontextprotocol/go-sdk`). WhatsApp lewat `internal/wa` (interface `Transport`; implementasi `whatsmeow` linked-device dan Cloud API).
- **Database**: PostgreSQL 16. Satu database, skema `public`; JSONB untuk payload sinyal dan kebijakan; `pg_trgm` untuk pencarian; partisi bulanan untuk `signals` dan `chat_messages` (dari Stage 13).
- **Frontend**: React 19 + TypeScript + Vite. Router `react-router`, data `@tanstack/react-query` + SSE, styling CSS Modules + token dari mockup (tanpa Tailwind), chart SVG buatan sendiri (mengikuti mockup), 3D `three` (r128+). Test `vitest` + `@testing-library/react`; e2e `playwright`.
- **Infra**: monorepo; Docker Compose (postgres, api, worker, web via Caddy); `.env` + `.env.example`. Target: Ubuntu VPS/mini PC, single-tenant, ≤ 50 pengguna.

## 5. Konvensi
- Struktur: `cmd/api`, `cmd/worker`, `cmd/arcctl` (CLI: migrate, seed, reanalyze), `internal/{domain,metrics,orchestrator,agents,llm,mcp,wa,odoo,api,store}`, `db/{migrations,queries,seed}`, `web/` (Vite), `infra/`, `docs/`.
- Domain murni Go tanpa dependensi DB/HTTP (`internal/domain`, `internal/metrics`): mudah diuji, rumus Orbit hidup di sini.
- `make dev`, `make check` (`go vet`, `golangci-lint`, `go test ./...`, `npm run typecheck`, `npm test`), `make migrate`, `make seed`, `make sqlc` selalu ada dan jalan.
- Test Go: unit untuk domain/metrics (tabel kasus dari glossary), integrasi dengan Postgres via `testcontainers-go` (atau `DATABASE_URL_TEST`). Kasus penting tertulis di tiap prompt tahap.
- API JSON `snake_case`, waktu RFC 3339 UTC, uang integer rupiah (`int64`), persentase integer 0–100.
- Konfigurasi lewat env; rahasia tidak pernah di-commit. Setiap panggilan LLM dicatat: provider, model, token, biaya estimasi, tujuan, hash input, `cycle_id`.
- Migrasi hanya lewat `goose`; tidak ada DDL dari kode aplikasi.
- Jangan menambah dependensi besar tanpa ADR. Prefer standard library.

## 6. Definition of Done (semua tahap)
- Acceptance criteria terpenuhi dan dibuktikan dengan perintah yang bisa diulang.
- `make check` hijau; `make dev` menyalakan postgres + api + worker + web tanpa error; `make seed` memuat 18 dealer contoh.
- `.env.example`, `README.md` (bagian tahap), `CHANGELOG.md` diperbarui.
- Tidak ada TODO tersembunyi: yang belum dibuat masuk `docs/OPEN-QUESTIONS.md` atau prompt tahap berikutnya.
- `.arc/progress.json` diperbarui dan di-commit.
