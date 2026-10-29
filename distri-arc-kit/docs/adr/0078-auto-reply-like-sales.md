# ADR 0078 — Chat otomatis: Lyra membalas semua chat seperti sales

Status: diterima · 2026-10-29 · Sam ("saya ingin otomatis tidak hanya stok tapi yang lain juga, menjawab seperti sales pada umumnya")

## Konteks
ADR 0077 membuat Lyra menjawab pertanyaan produk dengan daftar stok/harga yang kaku, dan pesan lain mendapat template
"Sales kami sedang istirahat". Sam ingin semua chat pada jam aktif dijawab sesuai isinya, dengan gaya sales.

## Keputusan
- Pengaturan baru `sales_style` (halaman Chat otomatis → langkah 2, "Lyra membalas SEMUA chat seperti sales").
- Untuk setiap pesan, sistem menghitung dulu faktanya: kartu produk Lyra (stok per kota, harga DPP sesuai tier) dan
  jawaban AI Trigger (status order, limit/tagihan, stok, harga). Lyra menerima pesan, 10 chat sebelumnya, fakta itu,
  dan jam sales kembali, lalu menulis balasan seperti sales (sesi `orbit-lyra-sales`).
- Nama pelanggan, nomor telepon, dan e-mail disamarkan sebelum ke Lyra. Lyra menulis `{nama}`, dan sistem menggantinya.
- **Cek angka**: setiap angka di balasan harus ada di fakta, pesan pelanggan, riwayat chat, atau jam kembali
  (`replyai.CheckDraft`). Balasan kosong, lebih dari 640 karakter, atau dengan angka lain tidak dikirim. Sebagai
  gantinya terkirim kartu produk, jawaban AI Trigger, atau template (urutan ADR 0077).
- Prompt melarang janji diskon, harga khusus, kredit/tempo, tanggal kirim, menahan stok, dan menerima pesanan. Untuk
  itu Lyra mengatakan sales akan menindaklanjuti. `perlu_sales` dicatat di log.
- Balasan diakhiri "_— Lyra, asisten GSI (pesan otomatis)_" supaya terbaca otomatis (`is_auto_reply`) dan chat
  tetap "Belum dibalas" untuk sales. Lyra tidak mengaku manusia.
- Batas Lyra 10 balasan per chat per jam aktif (template tetap 1, AI Trigger 5).

## Akibat
- Tetap mati sampai CEO menyalakan chat otomatis. Pengaturan lama yang sudah disimpan tanpa `sales_style` tetap
  memakai perilaku ADR 0077 sampai pilihan ini dicentang dan disimpan.
- Job `chat.auto_reply` bisa berjalan sampai ±10 menit (bacaan produk + balasan Lyra, masing-masing maksimal 4 menit).
- Lyra hanya membaca teks. Foto tanpa teks belum dibaca di chat otomatis.
