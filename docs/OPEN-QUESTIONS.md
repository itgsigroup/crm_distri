# Pertanyaan terbuka
Diisi Claude Code saat suatu tahap memakai mock atau butuh keputusan Sam. Format: tanggal · tahap · pertanyaan · dampak kalau tidak dijawab · default yang dipakai sementara.

## Kredensial (tahap selesai dengan mock sampai diisi di `.env`)
- **2026-10-04 · 02/04 · Nomor WhatsApp cadangan untuk uji QR.** Bridge (Baileys 7.0.0-rc14) sudah diuji sampai QR asli terbit dari server WhatsApp; uji kirim/terima dari HP lain belum dilakukan. Dampak: latensi ≤ 5 detik belum terbukti di nomor asli. Default: FakeTransport + replay fixture. Langkah: `docs/testing/whatsapp.md`.
- **2026-10-04 · 02 · WhatsApp Cloud API** (`WA_CLOUD_TOKEN`, `WA_CLOUD_PHONE_NUMBER_ID`, `WA_CLOUD_APP_SECRET`). Dampak: nomor bisnis resmi belum bisa dipakai; template di luar jendela 24 jam belum dibuat. Default: hanya transport bridge.
- **2026-10-04 · 03 · `ANTHROPIC_API_KEY`.** Dampak: ekstraksi, identifikasi, brief, dan Ask memakai FakeProvider deterministik (jawaban dari fixture); eval provider asli belum dijalankan. Default: `ARC_LLM_PROVIDER=auto` → fake. Setelah diisi: `make eval` lagi dan bandingkan `docs/eval/capture-anthropic.md`.
- **2026-10-04 · 03 · Truecaller Business API & penyedia web search** (`TRUECALLER_API_KEY`, `WEB_SEARCH_API_KEY`). Dampak: identifikasi nomor inbound memakai data fake `tests/fixtures/identity.json`. Pertanyaan: apakah GSI sudah punya kontrak Truecaller Business? (berbayar — perlu persetujuan Sam).
- **2026-10-04 · 03 · Getcontact.** Tidak ada API resmi; tersedia impor manual (tempel tag) dengan sumber `getcontact_manual`. Keputusan: tetap manual?
- **2026-10-04 · 08/10 · Odoo** (`ODOO_URL`, `ODOO_DB`, `ODOO_USERNAME`, `ODOO_API_KEY`, `ODOO_COMPANY_IDS`). Dampak: sinkron & tulis-balik memakai FakeOdoo (3 perusahaan [NEW], 8 opportunity). Butuh: user API khusus ARC dengan hak baca crm.lead/sale.order/account.move dan tulis terbatas (probability, mail.activity, message_post). **Penautan pertama ke data Odoo asli wajib ditinjau Sam sebelum dijalankan** (CLAUDE.md §3).
- **2026-10-04 · 09 · Google Workspace OAuth** (`GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`). Dampak: Gmail/Calendar memakai 12 email `.eml` + 3 event fixture. Butuh: OAuth client (Internal) di Google Cloud project GSI, scope `gmail.readonly`, `gmail.compose`, `calendar.readonly`.
- **2026-10-04 · 07 · SMTP / Basecamp** (`SMTP_*`, `BASECAMP_*`). Dampak: brief dan notifikasi dicatat oleh FakeNotifier (tabel `notifications`), tidak terkirim. Pertanyaan: kirim brief via email kantor atau Basecamp?
- **2026-10-04 · 12 · LPSE / e-katalog.** Tender radar memakai 3 tender fixture + impor manual. Scraping LPSE otomatis belum dibuat (ketentuan situs & stabilitas). Keputusan: langganan agregator tender (berbayar) atau impor manual mingguan?
- **2026-10-04 · 02 · Talenta (HR).** Registry nomor internal mendukung impor CSV ekspor Talenta; integrasi API belum. Default: CSV.

- **2026-10-04 · 02 · Baileys masih release candidate (7.0.0-rc14).** Dipin persis; pantau rilis stabil 7.x dan ikuti prosedur upgrade di runbook. Default: rc14.

## Keputusan bisnis yang perlu dikonfirmasi
- **Batas anti-blokir WhatsApp** (ADR 0004): 20/jam, 120/hari per nomor, jam tenang 21.00–07.00, hanya membalas kontak yang menghubungi lebih dulu. Konfirmasi apakah ada kebutuhan menghubungi kontak baru via WA (jika ya: Cloud API + template, bukan bridge).
- **Policy awal** (Pengaturan → Policy): batas diskon tanpa persetujuan CEO, plafon kredit, ambang "sunyi" (hari), benchmark L2C per tahap, target kuartal berikut (fixture: Rp 4,6 M). Nilai fixture dipakai sampai dikonfirmasi.
- **Grup WhatsApp opt-in**: daftar grup eksternal yang boleh dibaca ARC dan siapa yang menyetujui (go-live checklist).
- **Penerima brief** pagi 06.45 / sore 16.00 (default: CEO).
- **Retensi**: InboundContact yang tidak jadi lead dihapus setelah 90 hari (kebijakan privasi) — konfirmasi.

## Operasional (tahap 13)
- **Docker** tidak terpasang di mesin pengembangan; `infra/docker-compose.yml` divalidasi sintaksnya saja. Uji `make compose-up` + restore di mesin bersih dan 48 jam staging harus dilakukan di Mini PC/VPS target sebelum go-live (bukti dicatat di `docs/go-live-checklist.md`).
- **Backup offsite**: `scripts/backup.sh` membuat dump harian lokal (terenkripsi bila disk terenkripsi). Upload ke Google Drive menunggu kredensial Google; alternatif: `rclone` ke Drive dengan enkripsi `crypt`.
- **2026-10-04 · 13 · Patch Go toolchain.** `govulncheck ./...` melaporkan 17 kerentanan pustaka standar Go 1.26.2 (mis. `net/http`), semua diperbaiki di Go ≥ 1.26.3; dependensi pihak ketiga tidak terdampak pada jalur yang dipanggil. Image Docker (`golang:1.26-alpine`) sudah memakai patch terbaru; mesin pengembangan perlu upgrade Go sebelum build biner produksi di luar Docker. `npm audit`: 0 kerentanan.
