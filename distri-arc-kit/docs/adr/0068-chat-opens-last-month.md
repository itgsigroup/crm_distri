# ADR 0068 — Chat yang dibuka memuat 1 bulan terakhir

Status: diterima · 2026-10-27 · Sam

## Konteks
Membuka satu chat memuat semua pesan yang tersimpan di jendela chat (sampai 90 hari, maks. 3.000 pesan). Di laptop
yang lebih pelan (ThinkPad) chat panjang terasa berat: ribuan bubble digambar sekaligus. Sam meminta chat tidak
memuat data setahun, cukup 1 bulan.

## Keputusan
- `CHAT_OPEN_DAYS` (bawaan **30**): saat chat dibuka, `GET /chat/threads/{id}` mengirim pesan 30 hari terakhir saja
  (tetap di dalam `CHAT_WINDOW_DAYS`). Pesan lebih lama dimuat saat digulir ke atas, 500 per halaman, seperti sebelumnya.
- Chat yang 30 hari terakhir kosong tetap terbuka dengan 30 pesan terakhirnya (bukan halaman kosong).
- Daftar chat, Cari chat, Statistik, dan Report tidak berubah: tetap menjangkau seluruh pesan yang tersimpan.
- `0` mematikan batas ini (kembali memuat seluruh jendela chat).

## Konsekuensi
- Membuka chat lebih ringan dan cepat; pesan lama tetap ada dan bisa dimuat dengan menggulir.
- Melompat ke hasil pencarian lama tetap memuat semua pesan dari hasil itu sampai yang tampil, jadi percakapan bersambung.
