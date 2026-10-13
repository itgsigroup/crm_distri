# ADR 0028 — Riwayat WhatsApp satu tahun di halaman Chat

Status: diterima · 2026-10-13 · menggantikan bagian retensi chat ADR 0015 (90 → 365 hari)

## Konteks
Permintaan: halaman Chat langsung menampilkan pesan **satu tahun ke belakang** tanpa memuat tiap percakapan satu per
satu, dan ada filter **Belum dibaca** seperti WhatsApp. Sebelumnya: retensi chat 90 hari (purge tiap malam), bridge
hanya meneruskan riwayat 30 hari saat pairing (`syncFullHistory: false`, `history_days` 30), dan thread memuat 30 pesan
per klik "Muat pesan sebelumnya".

## Keputusan
- **Retensi** `retention.chat_days` 365 (default, seed, migrasi 0024 menaikkan nilai < 365 dengan versi + riwayat
  kebijakan). Tetap bisa diubah di Pengaturan (30–730). Sinyal, `llm_calls`, identifikasi tidak berubah.
- **Bridge** meneruskan riwayat minimal `BRIDGE_HISTORY_DAYS` (365) untuk semua sesi, termasuk yang sudah tertaut;
  meminta riwayat penuh saat pairing (`syncFullHistory: true`) dan menerima semua jenis sinkron riwayat (default
  Baileys membuang `FULL`). Nama perangkat tetap (anti-blokir, ADR 0017).
- **Backfill on-demand** untuk nomor yang sudah tertaut (tanpa scan ulang): job `wa.backfill` (worker, tiap 2 menit,
  maks. 20 permintaan per putaran) memilih percakapan nomor yang terhubung yang belum selesai, lalu meminta HP
  `fetchMessageHistory` 50 pesan sebelum pesan tertua yang tersimpan (`POST /sessions/{id}/history`). Jawaban HP
  datang sebagai event riwayat biasa (dedup per `wa_msg_id`, masuk sebagai sudah dibaca, tidak memicu identifikasi).
  Percakapan selesai bila pesan tertuanya ≥ jendela (min(`WA_BACKFILL_DAYS`, `retention.chat_days`)) atau 3 permintaan
  beruntun tidak membawa pesan lebih lama (`chat_threads.history_*`, migrasi 0024). Bridge memberi jeda 4 detik per
  nomor dan maks. 400 permintaan/jam; nomor terputus/terlalu cepat → dicoba putaran berikutnya. Chat 1-on-1 yang
  dialamatkan HP dengan LID diminta dengan LID-nya.
- **API** `GET /api/chat/threads/{id}`: halaman pertama memuat seluruh jendela (hingga 3000 pesan), dengan `has_more`,
  `history_from`, `history_days`; halaman lebih lama (500) hanya untuk chat yang sangat ramai, dimuat otomatis saat
  digulir ke atas — tanpa tombol.
- **UI**: chip *Belum dibaca* (lintas jenis, dengan jumlah); chat yang sedang dibuka tetap terlihat di filter itu
  sesudah dibaca, seperti WhatsApp.

## Konsekuensi
- Penyimpanan chat ±4× lebih besar (partisi bulanan tetap; purge membuang bulan utuh > 365 hari).
- Privasi (09-policies-security): chat disimpan 365 hari; aturan nomor internal, DM karyawan, dan grup internal tidak
  berubah — riwayat melewati ingest yang sama.
- Kelengkapan riwayat bergantung pada HP: WhatsApp hanya mengirim yang masih ada di HP, dan HP harus online.
- Hanya transport Baileys yang mendukung backfill on-demand; whatsmeow/Cloud API tidak (Cloud API memang tanpa riwayat).
