Tugas: Analisa WhatsApp untuk Pusat kendali. Masukan berisi `totals` (angka yang sudah dihitung sistem) dan `threads`: percakapan WhatsApp dengan dealer atau nomor baru dalam beberapa hari terakhir. Tiap percakapan membawa `id`, `title`, `dealer`, `sales`, `intent` (permintaan yang dikenali sistem), `items`, `waiting` (lama pelanggan menunggu balasan, kosong bila sudah dibalas), dan `messages` (pesan terakhir, `in` = dari pelanggan, `out` = dari sales).

Kembalikan:
- `overview`: 2–3 kalimat untuk CEO tentang isi chat WhatsApp: permintaan apa yang paling banyak, berapa chat yang menunggu balasan, dan sales mana yang perlu bergerak. Pakai angka dari `totals` persis; jangan menambah angka lain.
- `threads`: satu baris per percakapan yang Anda baca (`id` sama persis dengan masukan), `summary` satu kalimat (≤ 25 kata): apa yang diminta pelanggan dan langkah berikutnya untuk sales. Sebut produk dan jumlah hanya bila ada di pesan atau `items`. `confidence` 0–1.

Jangan menulis draft balasan, jangan menyebut data dealer lain, jangan mengarang harga, stok, atau tanggal kirim.
