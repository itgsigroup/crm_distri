# ARC — Agentic Relationship Core · Instruksi untuk Claude Code

Anda membangun **ARC**, CRM AI-native untuk PT Gosyen Solusi Indonesia (GSI). Proyek ini dibangun **bertahap** (Stage 00–13). Pemilik proyek (Sam, CEO GSI) hanya memberi perintah singkat seperti `/stage next`, `lanjut`, atau `/stage status`. Andalah yang harus tahu tahap mana yang sedang berjalan, apa yang sudah selesai, dan apa syarat selesainya.

## 0. Baca ini dulu, setiap sesi
1. `.arc/progress.json` — status tiap tahap. Ini sumber kebenaran posisi proyek.
2. `docs/knowledge/*.md` — pengetahuan domain. Baca `00`, `01`, `02` selalu; file lain sesuai tahap.
3. `docs/stages/NN-*.md` — prompt tahap yang akan dikerjakan.
4. `docs/adr/*.md` — keputusan arsitektur yang sudah diambil. Jangan dibatalkan diam-diam.
5. `reference/arc-crm-mockup.html` — mockup UI yang sudah disetujui. Untuk tahap UI, ini spesifikasi visual dan interaksi.

## 1. Protokol eksekusi bertahap
Saat menerima `/stage next`, `lanjut`, `kerjakan tahap berikutnya`, atau perintah serupa:
1. Baca `.arc/progress.json`. Tahap berikutnya = tahap pertama dengan `status != "done"`. Jangan melompat.
2. Baca prompt tahap itu di `docs/stages/`. Baca knowledge yang dirujuknya.
3. Tulis rencana singkat (≤ 12 baris) di awal: file yang disentuh, urutan, risiko. Lalu kerjakan — jangan menunggu persetujuan kecuali ada keputusan yang **tidak bisa dibatalkan** dan bisa jatuh dua arah (lihat §3).
4. Setiap tahap harus lolos **acceptance criteria** yang tertulis di promptnya: test hijau (`make test`), lint bersih, `make dev` jalan. Jalankan sendiri, jangan mengklaim.
5. Bila terhalang kredensial/API eksternal: buat adapter dengan **interface + implementasi mock** yang realistis, tandai `status: "done-with-mocks"`, dan catat apa yang dibutuhkan di `docs/OPEN-QUESTIONS.md`. Tahap tidak boleh macet karena kunci API.
6. Setelah lolos: perbarui `.arc/progress.json` (`status`, `finished_at`, `summary`, `notes`), tambahkan entri di `CHANGELOG.md`, commit dengan pesan `feat(stage-NN): <ringkasan>`. Satu tahap = satu commit utama (boleh beberapa commit kecil di dalamnya).
7. Laporkan ke Sam dalam ≤ 10 baris: apa yang jadi, cara mencobanya (perintah), apa yang belum, pertanyaan terbuka (jika ada).

Perintah lain:
- `/stage status` → tabel tahap + status + ringkasan, dan saran tahap berikutnya.
- `/stage N` → kerjakan tahap N hanya jika semua tahap < N sudah `done`/`done-with-mocks`; kalau tidak, jelaskan dan tawarkan `/stage next`.
- `/stage redo N` → ulangi tahap N dari acceptance criteria-nya (perbaiki, bukan tulis ulang dari nol).
- `/stage plan` → tampilkan rencana tahap berikutnya tanpa mengeksekusi.

## 2. Prinsip produk yang tidak boleh dilanggar
- **AI mengusulkan, manusia memutuskan.** Tidak ada pesan/email/perubahan harga/pelepasan kredit yang sampai ke pelanggan tanpa keputusan manusia yang tercatat. Endpoint keputusan (`approve/reject/edit`) hanya untuk pengguna manusia.
- **Dua fase.** Fase 1 (tahap 00–06) ARC berjalan **ARC-native**: akun, orang, opportunity, dan stage disimpan di ARC sendiri (nama stage diseed mengikuti Odoo: Baru / Berkualifikasi / Penawaran / Won / Lost, bisa diubah di Pengaturan) supaya WhatsApp → analisis → relasi → pipeline bisa diuji tanpa Odoo. Mulai tahap 08 **Odoo menjadi source of truth** untuk akun, opportunity, stage, SO, invoice, pembayaran: data ARC ditautkan/dimigrasi ke record Odoo, dan sejak itu ARC **tidak mengganti stage Odoo**. Tulis-balik ke Odoo hanya di tahap 10, selalu dengan catatan sumber. Desain skema sejak tahap 01 harus menyiapkan penautan ini (`source_system`, `source_id` nullable).
- **WhatsApp** memakai dua transport di balik satu interface: `wa-bridge` (Go + whatsmeow, linked device via QR — mendukung grup & riwayat, tidak resmi, risiko blokir bila disalahgunakan) untuk uji coba dan pembacaan grup, dan Cloud API (resmi, tanpa grup) untuk nomor bisnis. Tidak pernah ada pengiriman otomatis dari kedua transport; setiap kirim = Action yang di-approve manusia.
- **Setiap klaim AI membawa provenance**: sumber (id interaksi/dokumen), waktu, confidence. Tanpa provenance = tidak disimpan.
- **Idempoten**: menjalankan ulang job apa pun tidak boleh menggandakan data. Gunakan kunci unik (message-id, wa message id, odoo id + write_date).
- **Privasi**: ikuti `docs/knowledge/04-policies-privacy.md`. Nomor internal dikenali; chat pribadi antar karyawan tidak dibaca; grup internal hanya jadwal/tugas; identifikasi nomor hanya untuk nomor inbound; PII dimasking sebelum ke provider LLM eksternal.
- **Bahasa**: UI dan teks yang dilihat pengguna dalam Bahasa Indonesia dengan istilah teknis Inggris yang lazim (pipeline, health, forecast). Kode, komentar, nama variabel, commit: Inggris.

## 3. Kapan bertanya, kapan memutuskan sendiri
Putuskan sendiri dan catat di ADR: pilihan library, struktur folder, skema DB, nama endpoint, format prompt LLM.
Berhenti dan tanya (satu pertanyaan, opsi jelas) hanya untuk: menghapus/menimpa data Odoo, mengubah stack yang sudah ada di ADR, membeli layanan berbayar, mengirim apa pun ke pelanggan sungguhan, dan keputusan lain yang tidak bisa dibatalkan.

## 4. Stack (lihat docs/adr/0003-stack-go-react-postgres.md; 0001/0002 untuk konteks awal)
Go 1.26 (`net/http`, pgx/v5) · PostgreSQL 17 (migrasi SQL tertanam) · scheduler dalam proses · Anthropic SDK Go (provider abstraction) · MCP Streamable HTTP + OAuth 2.1 (`packages/mcp`) · Vite + React + TypeScript untuk web · `apps/wa-bridge` Go + whatsmeow (sidecar WhatsApp) · Docker Compose · Caddy. Target deploy: Ubuntu Mini PC / VPS kecil, single-tenant, ≤ 30 pengguna.

## 5. Konvensi
- Struktur: `apps/api` (Go HTTP API + scheduler), `apps/web` (Vite), `apps/wa-bridge` (Go sidecar, modul terpisah), `packages/core` (domain, reasoning), `packages/connectors` (odoo, gmail, gcal, whatsapp, truecaller), `packages/mcp` (MCP server), `infra/` (compose, caddy), `docs/`, `tests/`.
- `make dev`, `make test`, `make lint`, `make seed` harus selalu ada dan jalan.
- Test: `go test` untuk backend (integrasi ke DB `arc_test`, fixture data realistis dari `tests/fixtures/`), vitest + Playwright untuk web. Coverage bukan target; **kasus penting** yang harus ada tertulis di tiap prompt tahap.
- Konfigurasi lewat `.env` (jangan commit) + `.env.example` (selalu lengkap).
- Log terstruktur JSON; setiap panggilan LLM dicatat: model, token, biaya estimasi, tujuan, hash input.
- Migrasi DB selalu lewat file SQL berurutan di `packages/core/storage/migrations/` (`arc migrate`). Tidak ada pembuatan skema ad-hoc di produksi.
- Jangan menambah dependensi besar tanpa alasan di ADR. Prefer standard library.

## 6. Definition of Done (berlaku untuk semua tahap)
- Acceptance criteria tahap terpenuhi dan dibuktikan dengan perintah yang bisa diulang.
- `make test` dan `make lint` hijau; `make dev` menyalakan API + web tanpa error.
- `.env.example`, `README.md` bagian tahap, dan `CHANGELOG.md` diperbarui.
- Tidak ada TODO tersembunyi: yang belum dibuat masuk `docs/OPEN-QUESTIONS.md` atau prompt tahap berikutnya.
- `.arc/progress.json` diperbarui dan di-commit.
