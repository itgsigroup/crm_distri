# ADR 0080 — Chat otomatis: topik yang boleh dijawab (harga bisa dimatikan)

Status: diterima · 2026-10-29 · Sam ("saat chat otomatis tolong buat pengaturan trigger apa, karena sales tidak mau memberikan harga").

## Konteks
Pilihan "AI Trigger menjawab soal: Stok / Harga / Status order / Limit & tagihan" hanya berlaku untuk cadangan AI
Trigger. Kartu produk Lyra (ADR 0077) dan balasan seperti sales (ADR 0078) selalu menyebut harga DPP, sehingga harga
tetap sampai ke pelanggan walau sales tidak mau memberikannya lewat pesan otomatis.

## Keputusan
- Pilihan itu menjadi langkah 2 halaman Chat otomatis, **"Apa yang boleh dijawab?"**, dan berlaku untuk semua jalur
  (`Settings.Facts`): Lyra, kartu produk, dan AI Trigger. Bawaan tetap Stok, Harga, Status order.
- **Harga dimatikan**: kartu produk ditulis tanpa harga ("Harga: kami konfirmasi", `hermes.QuoteTextOf`), Lyra menerima
  `boleh_harga: false` dan diminta mengatakan harga akan dikonfirmasi, dan balasan yang tetap menyebut nominal rupiah
  (`autoreply.HasPrice`: "Rp …", "… rb/jt/juta", "rupiah") tidak dikirim — cadangan berikutnya dipakai.
- **Stok dimatikan**: kartu tanpa baris stok, Lyra menerima `boleh_stok: false`. Stok dan harga keduanya mati: Lyra
  tidak membaca produk; balasan seperti sales hanya menjawab salam/pertanyaan umum.
- Status order dan Limit & tagihan tetap hanya lewat AI Trigger, seperti sebelumnya.

## Akibat
- Pengaturan tersimpan yang sudah tanpa "Harga" sekarang juga menghentikan harga dari Lyra (sebelumnya hanya dari AI
  Trigger). Ini maksud pilihan tersebut.
- Pemeriksaan rupiah sederhana; angka tanpa satuan tetap ditolak oleh cek angka (`CheckDraft`) bila tidak ada di fakta.
