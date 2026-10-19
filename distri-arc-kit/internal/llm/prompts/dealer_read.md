Tugas: Analisa Dealer untuk halaman dealer. Masukan berisi `facts` tentang satu dealer — angka yang sudah dihitung sistem (status orbit, `cyc`, siklus order, hari sejak order, jadwal order, segmen, omzet, porsi belanja, PIC aktif, skor, kredit: limit/terpakai/status, `invoice_terbuka` dengan hari telat, `lewat_tempo_rp`, order terakhir) dan `chat_terakhir` (15 pesan WhatsApp terakhir dalam 30 hari, `dari`: pelanggan atau sales). `items` adalah baris yang harus Anda isi.

Kembalikan:
- `overview`: 2–3 kalimat untuk sales dan manager: kondisi dealer ini sekarang menurut angka dan isi chat (mis. order lewat jadwal, invoice telat, keluhan, menunggu proyek, aktif tanya stok).
- `lines` dengan `key` persis:
  - `status`: `note` satu kata — `bermasalah` (invoice lewat tempo / over limit, lewat jadwal jauh, ada keluhan yang belum selesai, atau chat pelanggan tak dibalas lama), `perlu_perhatian` (mulai melambat, limit tipis, chat menurun), atau `aman`. `confidence` 0–1.
  - `alasan_1` (wajib), `alasan_2`, `alasan_3` (boleh dilewati): satu kalimat ≤ 25 kata masing-masing, sebut angka atau kutipan chat singkat.
  - `wa`: draf pesan WhatsApp dari sales ke dealer ini, ≤ 300 karakter, Bahasa Indonesia sopan dan wajar — sesuai status: bila bermasalah, sapa dan selesaikan masalahnya (ingatkan invoice dengan nomor & jatuh tempo, tanyakan kendala, jawab keluhan); bila perlu perhatian, sapa dan tawarkan bantuan order sesuai jadwalnya; bila aman, sapaan ringan atau info produk yang relevan dari chat.

Jangan mengarang angka, nomor invoice, harga, stok, atau isi chat yang tidak ada di masukan. Placeholder seperti <PIC_1> atau <NO_1> biarkan apa adanya. Draf tidak terkirim sendiri — sales yang memutuskan dan mengirim dari Chat.
