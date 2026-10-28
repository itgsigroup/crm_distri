# ADR 0066 — Balas otomatis saat sales istirahat (sisi server, bawaan mati)

Status: diterima sebagian · 2026-10-28 · Sam

## Konteks
Pelanggan yang bertanya saat sales istirahat menunggu sampai jam istirahat selesai. Idenya: pada jam istirahat yang
ditentukan, pertanyaan stok, harga, atau status order dijawab dari data sistem, dan pesan lain mendapat template
"balasan otomatis".

## Keputusan
- Pengaturan disimpan di kebijakan `auto_reply`: aktif/tidak, jam istirahat per hari (di dalam jendela kirim WhatsApp
  otomatis), template, jenis fakta yang dijawab, nomor yang ikut, dan siapa yang menyetujuinya. **Bawaannya mati**
  (`enabled: false`). Fitur hanya berjalan bila diaktifkan **dan** disetujui oleh pemegang menu Pengaturan
  (`PUT /api/auto-reply`), yang dicatat sebagai `approved_profile`.
- Saat pesan pelanggan masuk dan aturannya aktif, worker menjalankan job `chat.auto_reply`. Setiap pesan dijawab paling
  banyak sekali (tabel `auto_replies`, migrasi 0066). Kirimannya tercatat sebagai keputusan manusia yang menyetujui aturan
  (proposal, `decided_by` = penyetuju) dan lewat outbox serta penjaga anti-blokir biasa.
- 28 Okt: hanya sisi server yang di-deploy. Halaman pengaturan (Pengaturan → Balas otomatis) belum dibuat, jadi belum
  ada yang bisa menyalakannya dari aplikasi. Sebelum dinyalakan, aturan dan template harus disetujui Sam.

## Akibat
- Deploy ini tidak mengubah apa pun bagi pelanggan dan tim sales. Tidak ada pesan otomatis sampai fitur diaktifkan.
- Sisa pekerjaan: halaman pengaturan dan persetujuan Sam atas aturan dan template.
