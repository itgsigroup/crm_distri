# ADR 0020 — Peran sepenuhnya custom, master cabang, WhatsApp: scan dulu lalu pilih pengguna

Status: diterima · 2026-10-07 · menggantikan sebagian ADR 0019 (peran berbasis 5 peran tetap; nomor diketik di master pengguna)

## Konteks
Di ADR 0019 peran masih bergantung pada lima peran tetap (ceo/admin/finance/sales/warehouse) dan nomor WhatsApp
diketik di master pengguna. GSI ingin: peran tanpa hardcode — tiap peran dicentang akses per halaman; master cabang;
nomor WhatsApp tidak diketik, tetapi didapat saat HP di-scan di Chat lalu dipilih penggunanya.

## Keputusan
1. **Peran custom** (`roles.scope`, `roles.policies`, migrasi 0016): halaman (14, dikelompokkan), cakupan data
   (`all` semua data / `own` hanya dealer & chat miliknya), jenis keputusan, hak kebijakan (setara CEO), boleh
   memegang WhatsApp. Peran bawaan hanyalah contoh awal — bisa diubah dan dihapus bila tidak dipakai.
2. **Base diturunkan, tidak dipilih**: kebijakan → `ceo`; data miliknya → `sales`; halaman Pengguna/Pengaturan →
   `admin`; selain itu → `finance`. Base disalin ke `users.role` (dan profil `sales_users.role`) sehingga semua
   pemeriksaan cakupan data di server tetap berlaku. Aturan keamanan di `access.Normalize`: hak kebijakan selalu
   semua data; peran "data miliknya" tidak bisa membuka halaman berisi data semua orang (Kredit, Pengguna, Peran,
   Cabang, Pengaturan); rilis kredit di atas limit hanya untuk pemegang hak kebijakan.
3. **Keputusan dari centang peran** (`Decider.Decide`): tidak lagi dibatasi daftar per peran tetap; akun tanpa baris
   peran (seed lama / CLI) memakai akses bawaan base-nya.
4. **Halaman dikunci di server** (`access.ScreenForPath`), termasuk `/api/users` (Pengguna) dan `/api/roles`
   (Peran). Hanya pemegang hak kebijakan yang mengubah peran; selalu ada minimal satu pengguna aktif dengan hak
   kebijakan.
5. **Master cabang** (`branches`): cabang pengguna, mapping cabang/gudang data impor dipilih dari master; ganti nama
   cabang ikut mengubah pengguna, dealer, dan mapping. Daftar cabang terbuka untuk semua pengguna (pilihan),
   perubahan hanya untuk pemegang halaman Cabang.
6. **WhatsApp: scan dulu, lalu pilih pengguna**: `POST /wa/links` membuat sesi tautan dengan id sendiri (`link-…`)
   sebelum nomor diketahui (QR, atau kode di HP yang butuh nomor). Saat WhatsApp melapor terhubung, nomor dicatat di
   `wa_numbers` dengan `session_id` sesi itu; bridge menyertakan nomor HP (`account`) di setiap event. Lalu
   `PUT /wa/numbers/{wa}/user` memberikan nomor ke satu pengguna (satu pengguna satu nomor; percakapan nomor itu
   mengikuti sales pemegangnya). Pengguna tanpa halaman Pengguna hanya bisa mengambil nomor yang belum dipegang
   untuk dirinya sendiri. Master pengguna tidak lagi punya isian nomor WhatsApp.

## Konsekuensi
- Menambah peran baru tidak butuh kode; menambah *cakupan data* baru tetap butuh kode.
- Nomor tertaut yang belum dipilih penggunanya tetap menerima pesan; percakapannya terlihat oleh peran semua data
  sampai nomor diberikan ke pengguna.
- Menautkan ulang nomor yang sama dari sesi baru memindahkan `session_id`; sesi lama di bridge sebaiknya dilepas.
