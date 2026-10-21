Tugas: AI Trigger di Chat — cari barang yang dimaksud pelanggan. Pelanggan (dealer, toko, atau installer) menulis pesan WhatsApp `pesan` yang menanyakan stok atau memesan. `kandidat` adalah barang di data stok GSI yang mungkin dimaksud (`sku`, `nama`, `stok` total semua cabang). `riwayat` adalah pesan sebelumnya (lama → baru; `pelanggan` atau `kami`).

Tentukan SEMUA barang yang ditanyakan atau dipesan di `pesan`, dalam urutan pelanggan menulisnya:
- Pilih hanya `sku` dari `kandidat`, persis sama. Jangan mengarang SKU.
- Cocokkan tipe/model dengan teliti: kode model (mis. X5S-8W, H8c), resolusi (2MP/4MP), jumlah channel (4Ch/8Ch), varian (PoE, 4G, Pro). "H8c 2mp" adalah barang H8c 2MP, bukan H8c 3MP atau 4G.
- Bila pesan tidak menyebut barang (mis. "stoknya ada?"), ambil barang dari `riwayat` yang terakhir dibicarakan.
- Bila pesan menyebut barang tetapi tipenya tidak cukup jelas untuk memilih satu (mis. "nvr ezviz 8ch" dan ada beberapa NVR 8ch), JANGAN menebak: kosongkan `items` untuk barang itu dan jelaskan di `catatan` tipe mana saja yang mungkin.
- `qty` = jumlah unit yang diminta untuk barang itu bila disebut (mis. "H8c 2mp : 8" → 8, "1 unit" → 1); 0 bila tidak disebut. Harga di pesan (mis. "@580rb") BUKAN jumlah.
- Bila tidak ada barang dari `kandidat` yang dimaksud, kembalikan `items` kosong.

Kembalikan JSON `{"items": [{"sku": "<sku>", "qty": <angka>}], "catatan": "<singkat, boleh kosong>"}`.
