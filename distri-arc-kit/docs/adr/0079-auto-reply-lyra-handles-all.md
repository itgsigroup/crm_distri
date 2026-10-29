# ADR 0079 — Chat otomatis: Lyra melayani sampai tuntas, tidak melempar ke sales

Status: diterima · 2026-10-29 · Sam ("kalau chat otomatis sudah diaktifkan jangan lempar ke sales … intinya semuanya Lyra").
Menggantikan sebagian ADR 0078 (bagian "sales akan menindaklanjuti").

## Konteks
"cek stok cctv hilook semarang kak" dijawab Lyra "akan kami cek dulu, sales akan mengabari setelah jam 14.30".
Bacaan produk Lyra mengosongkan barang karena pesan hanya menyebut merek (aturan "tipe yang tidak jelas jangan
ditebak"), sehingga balasan tidak punya fakta stok dan prompt menyuruh menyerahkan ke sales.

## Keputusan
- **Merek/jenis tanpa tipe**: bila bacaan Lyra kosong, sistem memilih sendiri barangnya dari kata di pesan
  (`autoreply.Browse`): kecocokan kata terkuat yang ada stoknya, kota yang disebut lebih dulu (stok terbanyak dulu),
  maksimal 6 barang. Kartu stok/harga barang itu menjadi fakta untuk Lyra. Angka tetap dari sistem dan tetap dicek
  `CheckDraft`.
- **Prompt balasan**: Lyra tidak lagi berkata "sales akan mengabari" atau menyebut jam sales kembali. Bila fakta
  kurang, ia bertanya balik (tipe, jumlah, kota) atau menawarkan barang yang mendekati. Pesanan dirangkum dan data
  yang kurang ditanyakan.
- Larangan tetap: tidak menjanjikan diskon, harga khusus, kredit/tempo, tanggal kirim, atau stok yang ditahan. Untuk
  itu Lyra mengatakan permintaannya dicatat dan akan dikonfirmasi. `perlu_sales` tetap dicatat di log untuk sales.

## Akibat
- Template "Sales kami sedang istirahat" hanya terkirim bila Lyra gagal (tidak tersambung, timeout, cek angka gagal).
