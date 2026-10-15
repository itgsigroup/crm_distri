# ADR 0039 — Chat memuat 14 hari terakhir

Tanggal: 2026-10-15 · Status: diterima · Mengubah sebagian ADR 0028 (riwayat chat)

## Konteks
Chat makin lambat (daftar 10–18 dtk, buka percakapan ±10 dtk) sejak nomor-nomor baru menyinkronkan riwayat
pada 14 Okt. Penyebabnya:
- daftar chat menghitung "menunggu sejak" dan pesan terakhir untuk ±3.900 percakapan setahun;
- percakapan dibuka dengan seluruh isinya (sampai 3.000 pesan);
- backfill berkala terus meminta riwayat sampai 365 hari dari HP (±1.200 permintaan/jam). Setiap batch yang masuk memicu ingest dan refetch di semua tab.

## Keputusan
- `CHAT_WINDOW_DAYS` (bawaan 14):
  - daftar chat hanya memuat percakapan yang aktif dalam jendela ini;
  - percakapan dibuka dengan pesan dalam jendela ini, dan pesan lebih lama dimuat saat pengguna menggulir ke atas;
  - percakapan tanpa pesan dalam jendela dibuka dengan 30 pesan terakhirnya.
- Unduhan riwayat dari HP (saat pairing dan backfill berkala) dibatasi ke jendela yang sama (`WA_BACKFILL_DAYS` dipotong ke `CHAT_WINDOW_DAYS`).
- Data lama **tidak dihapus**: retensi tetap `retention.chat_days` (365 hari, ADR 0028).
- MCP dan Orchestrator tetap membaca seluruh riwayat.

## Akibat
- Chat yang terakhir aktif lebih dari 14 hari lalu tidak muncul di daftar sampai ada pesan baru.
- Untuk menampilkannya lagi, naikkan `CHAT_WINDOW_DAYS`. Variabel ini sengaja tidak berawalan `WA_`, jadi mengubahnya tidak me-restart wa-bridge.
