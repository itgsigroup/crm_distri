# ADR 0060 — Chat disimpan 90 hari; pesan lama dimuat dari HP seperti WhatsApp Web

Status: diterima · 2026-10-22 · Sam · menggantikan retensi 365 hari dari ADR 0028

## Konteks
GSI Orbit menyimpan setiap pesan WhatsApp di database sendiri (932.597 pesan, 630 MB, ±5.200/hari). Server WhatsApp
tidak menyimpan riwayat; WhatsApp Web menyimpan salinan di browser dan memuat pesan lama dari HP saat digulir. Sam
meminta masa simpan diperpendek ke 90 hari, dengan syarat chat lama tetap bisa dimuat lagi seperti WhatsApp Web.

## Keputusan
- `retention.chat_days` 365 → **90** (migrasi 0062, versi kebijakan baru + riwayat; seed dan default kode juga 90).
  Pembersihan malam (02.30 WIB, setelah backup 02.00) menghapus pesan lebih tua dari 90 hari; backfill otomatis
  berhenti di 90 hari.
- **Muat pesan lama dari HP**: `POST /chat/threads/{id}/history` meminta HP pemegang nomor (lewat bridge) 50 pesan
  sebelum pesan tertua yang tersimpan — on-demand history sync, seperti WhatsApp Web menggulir ke atas. Di Chat: tombol
  di puncak percakapan; pesan yang datang ditampilkan di atas. Bridge tetap meneruskan riwayat sampai
  `BRIDGE_HISTORY_DAYS` (365) — batas jangkauan muat dari HP.
- Pesan yang dimuat ulang lebih tua dari 90 hari tersimpan sementara dan dibersihkan lagi malam berikutnya.

## Konsekuensi
- 636.452 pesan (68%) lebih tua dari 90 hari dihapus pada pembersihan pertama; tetap ada di backup terenkripsi 30 hari.
- Cari chat, Statistik, Analisa Claude, dan Report WhatsApp hanya menjangkau 90 hari tersimpan.
- Muat dari HP butuh HP pemegang nomor online dan nomor terhubung; dibatasi bridge (anti-blokir).
