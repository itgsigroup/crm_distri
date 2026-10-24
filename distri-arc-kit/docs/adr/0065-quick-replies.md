# ADR 0065 — Balasan cepat di Chat (seperti WhatsApp Business)

Status: diterima · 2026-10-24 · Sam

## Konteks
Sam (24 Okt): "tambahkan fitur pesan kilat seperti di WhatsApp" — yang dimaksud adalah **balasan cepat** WhatsApp
Business: teks yang sering dikirim (nomor rekening, alamat gudang, salam pembuka) disimpan dengan pintasan. Pengguna
mengetik `/` di kolom chat lalu memilih salah satunya.

## Keputusan
- Tabel `quick_replies` (migrasi 0063): `shortcut` unik tanpa memandang huruf besar/kecil (tanpa "/", huruf/angka/`-`/`_`,
  ≤ 24), `body` ≤ 2000 karakter (baris baru dipertahankan), pembuat, dan hitungan pakai (`uses`, untuk urutan "paling
  sering dipakai"). Maksimal 200 per tim.
- **Milik tim**, seperti satu akun WhatsApp Business yang dipakai bersama: siapa pun di Chat bisa menambah. Mengubah
  atau menghapus hanya bisa dilakukan pembuatnya atau admin. Semua perubahan tercatat di audit.
- API: `GET/POST /chat/quick-replies`, `PUT/DELETE /chat/quick-replies/{id}`, `POST /chat/quick-replies/{id}/use`.
- Composer: `/` di awal kata membuka daftar yang cocok (pintasan yang diawali teks yang diketik lebih dulu, lalu yang
  memuatnya di pintasan atau isi; maksimal 8). Pilih dengan ↑ ↓ lalu Enter/Tab, atau klik; Esc menutup. Teks terpilih
  **hanya mengisi kolom**, jadi pengiriman tetap lewat tombol Kirim manusia (prinsip §2). Kiriman itu tetap balasan
  *live* (ADR 0064) dan melewati penjaga anti-blokir.
- Kelola lewat 📎 → **Balasan cepat**, atau "Kelola balasan cepat" di bawah daftar `/`.
- Tanpa variabel (mis. `{nama}`), sama seperti WhatsApp Business. Bisa ditambahkan nanti bila perlu.

## Akibat
- Teks baku tim seragam dan cepat diketik. Tidak ada pesan yang terkirim tanpa tombol Kirim.
- Balasan cepat tidak disinkronkan ke HP. Ia hanya ada di GSI Orbit; daftar balasan cepat di aplikasi WhatsApp
  Business tidak bisa dibaca lewat perangkat tertaut.
