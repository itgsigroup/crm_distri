# ADR 0007 — WhatsApp: proses pemilik koneksi, aturan kirim, penyimpanan sesi
**Status**: diterima · 2026-10-05 · melengkapi ADR 0002

## Keputusan
1. **Worker memiliki koneksi WhatsApp** (`WA_TRANSPORT=fake|whatsmeow|cloudapi`): ingest streaming, `wa.pair` (QR) dan
   `outbox.send` berjalan di sana; API hanya menulis outbox/job dalam transaksi yang sama (river insert-only client).
   Cloud API masuk lewat webhook di API (`/api/wa/cloud/webhook`, tanda tangan `X-Hub-Signature-256` wajib).
2. **Aturan kirim** (`internal/outbox`): pesan proaktif (follow-up, penagihan, push) dibatasi
   `followup.rules.max_per_day_per_sales` per nomor per hari (lewat batas → job di-snooze ke 08.00 besok) dan diberi
   jeda acak `WA_SEND_GAP` (default 20–90 dtk) antar kirim nomor yang sama. **Balasan manusia** di dalam percakapan
   (`kind=reply`) tidak memakan kuota proaktif; hanya jeda mengetik `WA_REPLY_DELAY` (2–6 dtk). Tidak ada broadcast.
3. **Balasan = keputusan**: `POST /chat/threads/{id}/messages` membuat proposal `reply` berstatus `approved` dengan
   `decided_by` = sales pengirim, provenance = sinyal pesan terakhir thread, baris `outbox`, pesan `pending` di thread,
   dan `audit_log`. Status `sent` + `wa_msg_id` diisi worker setelah transport mengirim.
4. **Store whatsmeow** di skema `whatsmeow` database yang sama; tabelnya dibuat oleh upgrade whatsmeow yang dijalankan
   `arc ctl migrate` (bukan saat runtime). Enkripsi kunci sesi dengan `WA_SESSION_KEY` belum tersedia di store resmi
   whatsmeow → dicatat di OPEN-QUESTIONS; mitigasi sementara: skema terpisah + backup terenkripsi (Stage 13).
5. **Seed chat**: hanya 6 thread mockup yang menjadi chat; riwayat WA sintetis lain tetap sebagai sinyal.
