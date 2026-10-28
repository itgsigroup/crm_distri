# ADR 0070 — Lyra di bawah bubble: stok per kota dan harga DPP dari Hermes Agent

Status: diterima · 2026-10-28 · Sam

## Konteks
Hermes Agent perusahaan (bernama **Lyra**) sudah tersambung ke GSI Orbit (ADR 0067). Permintaan Sam (28 Okt): setiap bubble
chat yang menanyakan produk, baik berupa tulisan maupun gambar, diberi hasil dari Lyra di bawahnya. Isinya jumlah stok tiap
gudang per kota dan harga per barang (DPP). Di bawahnya ada input diskon dalam persen yang langsung menghitung ulang harga dan
total. Bagian ini bisa diklik untuk dikirim ke pelanggan, tetapi pesannya bisa diedit dan dikonfirmasi dulu sebelum terkirim.

## Keputusan
- **Lyra memilih barangnya, sedangkan angkanya dihitung GSI Orbit** (prinsip §2: angka dari Go, bukan LLM). Lyra menerima
  teks pesan, foto (bila ada), 10 baris riwayat, dan ≤ 40 kandidat katalog (`replyrule.Candidates`). Lyra hanya menjawab
  JSON `{tanya_produk, items:[{sku, nama, qty}], catatan}`. SKU harus dikenal katalog (stok + `products`). Bila SKU kosong,
  nama dicocokkan lewat kode model yang sama. Selain itu barang dicatat sebagai "tidak ada di katalog". Lyra diminta tidak
  memanggil tool dan tidak menulis harga atau stok.
- Bacaan disimpan sekali per pesan di `hermes_product_reads` (migrasi 0068, kunci `message_id`), jadi membuka chat
  berulang tidak bertanya lagi (idempoten). Stok dan harga dihitung saat chat ditampilkan (`hermes.BuildCard`), sehingga
  selalu mengikuti sinkron stok terakhir.
- **Stok per kota dan gudang**: per kota diambil dari `stock_items.branch` (kota/cabang hasil mapping gudang). Rincian
  gudang diambil dari impor stok yang distaging (`import_rows` entity `stock`) dan dipetakan ke kota lewat mapping
  `warehouse`, sama seperti importer. Rincian gudang hanya tampil bila satu kota punya beberapa gudang atau nama gudangnya
  berbeda dari nama kota. Kota dengan stok 0 tidak ditampilkan.
- **Harga DPP** = harga tier dealer di chat (`products.prices[tier]`), atau harga list (tier A) bila chat belum tertaut ke
  dealer. Harga produk hasil impor diambil dari harga satuan baris faktur terakhir (tanpa PPN), sehingga dianggap DPP.
- **Pemicu**: saat chat pelanggan dibuka atau diperbarui (SSE), pesan masuk ≤ 3 hari yang belum dibaca dan memenuhi syarat
  dibaca otomatis. Maksimal 3 pesan per pemuatan, mulai dari yang terbaru. Syaratnya: pesan berupa foto, atau teks yang
  menyebut barang di stok disertai kata tanya/pesan (`stok`, `harga`, `ready`, `berapa`, `unit` …) atau kode model
  (`x5s`, `2mp`). Pesan lain dalam 7 hari bisa dikirim ke Lyra lewat menu bubble **Tanya Lyra (stok & harga)**. Kartu yang
  sudah ada bisa diminta **Baca ulang**. Chat internal tidak pernah dibaca.
- Lyra dipanggil asinkron dari proses API (sama seperti chat Hermes), dengan maksimal 2 bacaan berjalan bersamaan dan timeout
  10 menit. Bacaan yang tertunda lebih dari 10 menit dianggap gagal dan bisa dibaca ulang. Selesai membaca, server
  mengirim event `chat_message` sehingga kartu langsung berganti.
- Foto dikirim sebagai OpenAI content part `image_url` (data URL, maks. 4 MB). Bila model Hermes menolak foto, Lyra membaca
  teksnya saja dan kartu memberi keterangan. Teks dan riwayat dimasking (nomor, email, NIK, rekening) sebelum dikirim.
  Foto tidak bisa dimasking.
- **Diskon dan pengiriman** dilakukan di browser. Satu input diskon (%) per kartu, dengan qty per barang yang bisa diubah
  (awalnya jumlah yang diminta pelanggan, atau 1). Harga satuan setelah diskon dibulatkan ke rupiah, dan total = harga
  setelah diskon × qty. **Kirim ke pelanggan** membuka sheet berisi pesan yang bisa diedit (format WhatsApp, "Harga DPP,
  belum termasuk PPN"). Pesan baru masuk antrean kirim biasa (`POST /chat/threads/{id}/messages`, balasan ke bubble itu,
  `sent_by` tercatat, lewat penjaga anti-blokir) setelah tombol **Kirim** ditekan. Tidak ada yang terkirim otomatis.
- Tanpa Hermes yang tersambung, kartu berjalan dalam **mode demo**: barang dipilih dari kata di pesan, dan foto tidak bisa dibaca.

## Akibat
- Sales langsung melihat stok per kota/gudang dan harga DPP di bawah pertanyaan pelanggan, lalu bisa memberi diskon dan
  mengirim penawaran dalam beberapa klik.
- Setiap foto atau pertanyaan produk memicu satu run Hermes di device Lyra. Isi pesan dan foto ikut terkirim ke provider
  model yang dipakai Hermes.
- Belum ada pemicu dari worker saat pesan masuk. Pesan baru dibaca begitu chat-nya dibuka. Bila nanti diperlukan, worker
  bisa memanggil `hermes.Service.ReadProducts` dari `ingest.OnInbound`.
