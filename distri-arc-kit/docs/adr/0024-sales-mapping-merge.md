# ADR 0024 — Mapping sales: satu Pengguna ↔ banyak nama sales BigQuery

Status: diterima · 2026-10-09

## Konteks
Impor BigQuery (Accurate) membuat satu profil `sales_users` per nama/kode sales di sumber. Sumber mengeja orang yang
sama beberapa kali ("Granike Monica · Semua cabang", "Granike Monica M. · Semarang"; "Elisa" dua kali), sehingga
pilihan sales di Pusat kendali/Orchestrator penuh duplikat dan dealer seorang sales terpecah.

## Keputusan
- Relasi **one-to-many**: satu **Pengguna** (akun login, `users`) memegang banyak nama sales BigQuery. Setiap
  pengguna punya satu profil sales utama (`users.sales_user_id`); bila belum ada, profil dibuat saat nama pertama
  dihubungkan (nama = nama pengguna, cabang = cabang salah satu nama BigQuery). Nama BigQuery dihubungkan dengan
  menggabungkannya ke profil utama itu (`merged_into`), sehingga dealer seluruh nama tersebut menjadi milik
  pengguna dan cakupan data sales (Pusat kendali, Orchestrator, Orbit) otomatis mencakup semuanya.
- Profil utama pengguna lain tidak bisa diambil (400) — lepas dulu dari pengguna itu.
- Kolom `sales_users.merged_into` (migrasi 0019). Profil sumber yang digabung tetap ada (kunci impor `source_id`),
  tetapi `active = false` dan tidak muncul di daftar sales mana pun (`ListSalesUsers` hanya `active`, `ListTeam`
  tanpa yang digabung).
- Halaman **Master data → Mapping sales** (`GET /sales-map` → nama + `user_id`/`main`, dan daftar pengguna;
  `POST /sales-map/merge {sources, user_id}` menghubungkan, `{sources, target_id: null}` melepas), untuk
  pemegang menu Pengguna atau Pengaturan. Menggabung = dalam satu transaksi memindahkan dealer (`owner_id`),
  percakapan, nomor WA, sinyal, interaksi bulanan, login pengguna, dan nomor utama ke sales tujuan; `data_mappings`
  (kind `sales`) untuk kode & nama sumber diarahkan ke tujuan. Keputusan lama (`decided_by`, `confirmed_by`) tetap
  mencatat siapa yang memutuskan.
- Impor berikutnya menghormati gabungan: ejaan yang mapping-nya menunjuk sales lain ditandai `merged_into` lagi, dan
  pencocokan nama memakai sales tujuan.
- Memisah lagi (`target_id: null`) mengaktifkan profil, mengarahkan mapping ke dirinya, lalu memproses ulang data
  impor agar dealernya kembali.

## Konsekuensi
Tidak ada data Odoo/Accurate yang diubah; hanya salinan di GSI Orbit. Menggabung bisa dibatalkan.
