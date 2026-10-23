# ADR 0061 — Cadangan chat dalam format ekspor WhatsApp

Status: diterima · 2026-10-23 · Sam

## Konteks
Sejak ADR 0060 chat hanya disimpan 90 hari; backup database terenkripsi (pg_dump, 30 hari) tidak bisa dibaca tanpa
memulihkan database. Sam meminta fitur cadangkan pesan seperti di WhatsApp, dalam format WhatsApp, dan "sangat enteng".

## Keputusan
- **Format** = Ekspor chat WhatsApp Android berbahasa Indonesia, tanpa media: `23/10/26 14.05 - Nama: pesan` (WIB),
  pesan multi-baris berlanjut di baris berikutnya, `<Media tidak disertakan>` (+ caption di baris berikutnya; dokumen
  diawali nama filenya), `Pesan ini telah dihapus`, `<Pesan ini diedit>`. Baris pertama adalah baris sistem (tanpa
  `Nama:`) yang menyebut asal salinan. Nama: pesan keluar = nama nomor GSI; chat pribadi = nama chat seperti di Chat
  (`panggilan - nama`); grup = nama anggota, kalau tidak ada nomornya. Pesan `pending`/`failed` tidak ikut (tidak
  pernah sampai ke WhatsApp). Kredensial tetap disamarkan (`MaskCredentials`), sama seperti tampilan CRM.
- **Ekspor chat** (`GET /api/chat/threads/{id}/export`): siapa pun yang boleh membaca chat itu — seperti di HP;
  `Chat WhatsApp dengan <nama>.txt`, semua pesan yang tersimpan; grup = pesan dari semua nomor GSI di grup itu.
  Diaudit `chat.export`.
- **Cadangan chat** (`internal/chatbackup`, job `chat.backup` 02.15 WIB — setelah backup DB 02.00, sebelum
  pembersihan 02.30): `CHAT_BACKUP_DIR/chat-YYYY-MM.zip`, isi `<nomor GSI>/Chat WhatsApp dengan <nama>.txt`, satu per
  chat. Tiap malam ditulis ulang hanya **bulan berjalan dan bulan kemarin** (dua di tanggal 1), plus bulan tersimpan
  yang belum punya file. Bulan yang sudah lengkap **tidak pernah ditulis ulang**, jadi file tetap utuh setelah
  pembersihan 90 hari mengosongkannya. Rotasi `CHAT_BACKUP_KEEP_MONTHS` (12, sejalan dengan riwayat 365 hari yang
  bisa dimuat dari HP; 0 = selamanya).
- **Enteng**: pesan dibaca baris demi baris dari kursor PostgreSQL langsung ke zip (memori tetap), satu partisi bulan
  per malam, tanpa media; ditulis ke file sementara lalu di-rename (unduhan tidak pernah melihat file setengah jadi).
  Ringkasan (jumlah chat/pesan) di komentar zip, jadi daftar cadangan tidak membuka isi chat. Tanpa tabel baru.
- Pengaturan → **Cadangan chat** (CEO/admin): cadangan terakhir, **Cadangkan** (`POST /api/chat/backups`), daftar per
  bulan dengan **Unduh** (`GET /api/chat/backups/{name}`, diaudit `chat.backup.download`).
- **Privasi**: tiap entri zip membawa JID chat-nya di komentar entri. Chat yang ditandai **Obrolan pribadi** dan
  penghapusan UU PDP (`arc ctl pdp delete`) membuang chat satu-satu itu dari semua bulan (entri lain disalin tanpa
  kompresi ulang). File 0640 milik `arc` di `/var/lib/distri-arc/chat-backup` (satu-satunya folder yang boleh ditulis
  api/worker di systemd); tidak disalin ke luar server.

## Konsekuensi
- Diukur: 150 ribu pesan / 3.000 chat (±1 bulan produksi) → zip 2,3 MB dalam 1,6 detik, memori ±10 MB; 12 bulan ≈ 30 MB.
- `llm.MaskCredentials` kini melewati regex bila pesan tidak memuat kata kunci (pass, pwd, sandi, pin, otp, kode,
  token): 52 µs → 0,8 µs per pesan — tanpa itu satu bulan butuh 8 detik. Tampilan Chat ikut lebih ringan.
- Pesan yang dimuat ulang dari HP untuk bulan yang sudah lengkap tidak masuk cadangan bulan itu.
- Penghapusan UU PDP membuang chat pribadi orang itu, tetapi baris pesannya di **grup** tetap ada di cadangan bulan
  lama sampai rotasi (sama seperti backup database terenkripsi 30 hari) — dicatat di OPEN-QUESTIONS.
