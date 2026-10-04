# 03 — Integrasi (baca di tahap 02, 03, 08, 09, 11, 12)

## Odoo (XML-RPC, Odoo 16/17 SaaS)
Endpoint: `https://<db>.odoo.com/xmlrpc/2/common` (authenticate) dan `/xmlrpc/2/object` (`execute_kw`). Gunakan `xmlrpc.client` standar. Autentikasi: username + API key (bukan password). Multi-company: set `context: {allowed_company_ids: [...]}`; sinkron semua company `[NEW]`.
Model yang dipakai:
- `res.partner` (akun & kontak: `is_company`, `parent_id`, `phone`, `mobile`, `email`, `category_id`, `user_id`, `company_id`)
- `crm.stage` (nama stage — **ambil dari sini**, jangan hardcode), `crm.lead` (`type=opportunity`, `partner_id`, `stage_id`, `expected_revenue`, `probability`, `date_deadline`, `tag_ids`, `priority`, `activity_ids`, `user_id`, `team_id`, `write_date`), `mail.message` (chatter) untuk backfill riwayat
- `sale.order` (`state`, `date_order`, `partner_id`, `opportunity_id`, `amount_total`, field tanggal custom lead-to-cash yang sudah dibuat di Odoo — cek nama field di runtime dengan `fields_get`)
- `project.project`, `project.task` (stage: Project Selesai, SO-SPK-BAST, Invoice, Lunas — cek runtime), `account.move` (invoice: `invoice_date`, `invoice_date_due`, `amount_residual`, `payment_state`), `account.payment`
- `mail.activity` (untuk tulis-balik komitmen/tindakan di Stage 08), `res.users`
Sinkron: incremental by `write_date > watermark`, batch 200, backoff pada error. Tulis-balik hanya `crm.lead.probability` (opsional), `mail.activity`, `mail.message` (note), `crm.lead` create untuk lead baru — semuanya dengan teks "via ARC · sumber: …".

## Gmail & Google Calendar
OAuth 2.0 per pengguna (scope `gmail.readonly`, `gmail.compose` untuk draft, `calendar.readonly`). Ingest incremental dengan `historyId`/watermark; simpan `message-id`, `thread-id`, headers, body text (HTML → teks), lampiran (nama, mime, size; unduh hanya PDF/DOCX penawaran/proposal/PO/BAST). Draft balasan dibuat sebagai **Gmail draft**, tidak pernah `send` dari ARC.

## WhatsApp (lihat ADR 0002)
Dua transport di balik `WhatsAppTransport` dengan `WaEvent` standar:
- **wa-bridge (Baileys, linked device via QR)** — mendukung 1:1, **grup** (anggota, nama), riwayat awal, profil kontak. Tidak resmi: pin versi, reconnect otomatis, alert bila sesi putus; kirim hanya dari Action yang di-approve, rate-limit. Multi-sesi (satu per nomor). Berjalan sebagai sidecar Node di server GSI.
- **Cloud API (resmi Meta)** — 1:1 saja, tidak ada grup, tidak ada riwayat sebelum tersambung; template untuk > 24 jam; webhook dengan verifikasi signature. Untuk nomor bisnis resmi.
Grup: klasifikasi `external` (ada anggota non-internal) / `internal`; hanya grup yang di-opt-in yang dibaca. Chat 1:1 antar dua nomor internal tidak disimpan isinya. Backfill Cloud API via ekspor chat (.txt/.zip).

## Identifikasi nomor inbound
Urutan sumber: (1) Odoo `res.partner` phone match, (2) riwayat ARC, (3) profil WhatsApp Business (`contacts`/profile via API), (4) Truecaller Business API (nama, label spam), (5) Getcontact — **tidak ada API resmi**: dukung *impor manual* (screenshot/teks tag yang ditempel sales) dan tandai sumber `getcontact_manual`; jangan bangun klien protokol tidak resmi. (6) Research agent: web publik (domain perusahaan, berita). Gabungkan ≥ 2 sumber → confidence; simpan 90 hari kecuali jadi lead.

## Tender
LPSE per daerah tidak punya API seragam; gunakan scraper sopan (rate-limit, robots) atau layanan agregator bila Sam setuju (ADR). Fase awal: impor manual CSV + kata kunci; Research agent menilai kecocokan.

## Basecamp
API 3 (OAuth). Dipakai untuk: kirim brief ke Message Board/Campfire, buat To-do untuk tugas yang terdeteksi dari grup project.

## Talenta
Tidak ada API publik yang stabil untuk semua paket; dukung ekspor CSV karyawan (nama, unit, cabang, nomor) untuk registry nomor internal.

## LLM providers
Anthropic (Claude) sebagai default: tier `light` → Haiku 4.5, `heavy` → Sonnet 5, `interactive` → Sonnet 5; OpenAI dan Ollama sebagai implementasi `LLMProvider` lain. Masking PII sebelum kirim (nomor telepon, email pribadi, rekening → token `[PHONE_1]`, dst., dipulihkan setelah respons). Semua panggilan dicatat di `LLMCall`.
