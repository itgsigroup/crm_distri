# ADR 0002 — Backend WhatsApp
Status: diterima (tahap 00); implementasi bridge diganti Go + whatsmeow oleh ADR 0003 (fungsi sama).

## Konteks
GSI butuh membaca chat pelanggan **dan grup project** (Cloud API resmi Meta tidak mendukung grup dan tidak memberi riwayat sebelum tersambung), ingin menguji cepat dengan nomor yang ada, dan pada akhirnya ingin jalur resmi untuk nomor bisnis.

## Keputusan
- Satu interface Python `WhatsAppTransport` dengan **event schema standar** (`WaEvent`: `wamid`, `from`, `to`, `chat_id`, `is_group`, `group_meta`, `sender_name`, `text`, `media_meta`, `timestamp`, `transport`).
- **Transport A — `wa-bridge`** (`apps/wa-bridge`, Node 20 + TypeScript + `@whiskeysockets/baileys`): multi-sesi (satu sesi per nomor sales), pairing via QR yang ditampilkan di UI Pengaturan, auth state disimpan terenkripsi di `data/wa-sessions/`, menerima pesan 1:1 & grup, sinkron riwayat awal (30–180 hari sesuai pengaturan), metadata grup (anggota), nama profil kontak; meneruskan `WaEvent` ke FastAPI `POST /webhooks/wa` dengan HMAC; menerima perintah kirim hanya dari FastAPI dengan `action_id` yang sudah `approved`. Rate-limit kirim (≤ 20/jam/nomor), jeda acak, tidak pernah broadcast.
- **Transport B — Cloud API** (resmi): webhook `POST /webhooks/wa-cloud` (verifikasi signature), kirim via Graph API, template untuk > 24 jam; hanya 1:1.
- Pilihan transport per nomor di Pengaturan. Default uji coba: bridge pada nomor cadangan. Produksi: bridge untuk grup project (nomor khusus "ARC Project" yang dimasukkan ke grup), Cloud API untuk nomor bisnis cabang.
- Backfill riwayat untuk Cloud API: parser ekspor chat (.txt/.zip).

## Konsekuensi
Bridge memakai protokol tidak resmi: bisa putus saat WhatsApp berubah (pin versi Baileys, health-check reconnect, alert) dan berisiko blokir bila dipakai mengirim massal — karena itu kirim selalu manual-approve dan dibatasi. Data tidak pernah lewat pihak ketiga: bridge berjalan di server GSI sendiri.
