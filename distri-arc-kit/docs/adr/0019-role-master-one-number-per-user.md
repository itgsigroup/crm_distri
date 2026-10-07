# ADR 0019 — Master peran & akses; satu pengguna memegang satu nomor WhatsApp

Status: diterima · 2026-10-07

## Konteks
Peran pengguna tertanam di kode (ceo, admin, finance, sales, warehouse): menu, keputusan, dan cakupan data. GSI
butuh peran sendiri (mis. "Sales Telemarketing", "CS Kantor", "Gudang Medan") yang diatur dari Pengaturan, dan ke
depan setiap pengguna memegang tepat satu nomor WhatsApp yang dipakai di Chat. Nomor di Chat sebelumnya diketik
bebas (label + pemilik opsional).

## Keputusan
1. **Tabel `roles`** (migrasi 0015): `key`, `name`, `description`, `base` (salah satu dari 5 peran sistem),
   `screens` dan `decide` (null = semua milik base), `wa_allowed`, `system`, `active`. Lima peran bawaan diisi.
2. **Peran hanya mempersempit base-nya, tidak pernah melebarkan.** `users.role` tetap berisi base — semua pemeriksaan
   server yang ada (cakupan data sales, hak keputusan per jenis, Pengaturan untuk CEO/admin) tidak berubah;
   `users.role_key` menunjuk peran. Menu efektif = pilihan ∩ menu base; keputusan efektif = pilihan ∩ keputusan base
   (`internal/access`). Peran CEO tidak bisa dipersempit, base CEO tidak bisa dipakai peran baru, dan harus selalu ada
   satu CEO aktif.
3. **Ditegakkan di server**, bukan hanya menyembunyikan menu: route API milik layar tertentu (`/chat`, `/wa/groups`,
   `/wa/pair` → Chat; `/credit` → Kredit; `/relasi` → Peta relasi; `/stock/aging|critical|sales-by-product` → Push
   stok) ditolak 403 bila menu itu tidak dibuka peran; keputusan di luar `decide` ditolak di `proposals.authorize`.
   Bacaan bersama (dealer, proposal, `/stock/push`, `/dealers/credit-tight` di Pusat kendali) tetap terbuka.
4. **Hanya CEO yang mengubah master peran**; admin melihat dan menetapkan peran ke pengguna (peran ber-base CEO hanya
   oleh CEO).
5. **Satu pengguna, satu nomor**: `users.wa_number` (unik) di master pengguna; `wa_numbers.user_id` (unik). Tambah
   nomor di Chat memilih pengguna dari master — nomornya diambil dari sana (bila kosong boleh diisi sekali dan
   disimpan ke pengguna). Pengguna non-admin bisa menautkan nomornya sendiri. Nomor yang sedang tertaut tidak bisa
   diganti di master sebelum dilepas di Chat; peran tanpa WhatsApp tidak bisa memegang nomor.

## Konsekuensi
- Menambah peran tidak butuh deploy; menambah *jenis* akses data baru tetap butuh kode (base baru).
- Nomor tim tanpa pengguna tidak lagi bisa dibuat dari UI — buat pengguna untuknya (mis. "CS Kantor", peran Admin
  atau peran khusus).
- Akun lama tanpa `role_key` (seed/CLI) diperlakukan sebagai peran bawaan base-nya.
