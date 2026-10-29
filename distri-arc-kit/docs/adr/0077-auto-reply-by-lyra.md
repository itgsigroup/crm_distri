# ADR 0077 — Chat otomatis dijawab Lyra, dengan halaman pengaturannya

Status: diterima · 2026-10-29 · Sam ("chat otomatis dijawab oleh agent Hermes Lyra kita dan ada setting jam berapa sampai jam berapa")

## Konteks
ADR 0066 memasang mesin balas otomatis (bawaan mati) tanpa halaman. Pertanyaan fakta dijawab draf AI Trigger. Sam
ingin pertanyaan pelanggan dijawab Lyra (Hermes Agent perusahaan), dengan jam aktif yang bisa diatur.

## Keputusan
- Halaman **Pengaturan → Chat otomatis** (`/pengaturan/balas-otomatis`, pemegang menu Pengaturan membaca, CEO
  mengubah). Isinya: aktif/mati, Lyra nyala/mati, fakta cadangan AI Trigger, jam aktif per hari (mulai–selesai, di
  dalam jendela kirim otomatis 08.00–18.00 WIB), template, nomor yang ikut, dan riwayat 30 pesan otomatis terakhir.
  Tombol **Simpan & setujui** = persetujuan CEO (ADR 0066).
- Urutan jawaban pada jam aktif: (1) pertanyaan produk (kata kunci stok/harga/produk) dibaca **Lyra**. Lyra hanya
  menyebut SKU dan jumlah. Stok per kota dan harga DPP sesuai tier dealer dihitung sistem (`hermes.BuildCard`), dan
  pesannya sama dengan kartu Lyra tanpa diskon (`hermes.QuoteText`), ditandai "(Pesan otomatis dari Lyra …)".
  (2) Lyra tidak menemukan produk, belum tersambung, atau melewati 4 menit: draf AI Trigger. (3) Selain itu: template.
- Bacaan Lyra disimpan di `hermes_product_reads` (sama dengan kartu di Chat), jadi kartu yang sama tampil untuk sales
  dan tidak dibaca dua kali. Mode demo (Lyra belum tersambung) tidak pernah dikirim ke pelanggan.
- Setelah Lyra menjawab, dicek lagi apakah sales sudah membalas selama menunggu; bila ya, tidak dikirim.
- Jenis `lyra` di `auto_replies` (migrasi 0073), batas sama dengan jawaban fakta (5× per chat per jam aktif).

## Akibat
- Fitur tetap mati sampai CEO menyalakan dan menyimpannya di halaman.
- Jam di luar 08.00–18.00 tidak bisa dipilih. Membukanya berarti mengubah jendela kirim anti-blokir (ADR 0004), yang
  perlu keputusan terpisah.
- Job `chat.auto_reply` bisa berjalan sampai ±6 menit (menunggu Lyra). Jawaban yang lewat 15 menit dari pesan masuk
  tidak dikirim.
