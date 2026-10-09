# ADR 0024 — Mapping sales: menggabungkan ejaan sales dari data sumber

Status: diterima · 2026-10-09

## Konteks
Impor BigQuery (Accurate) membuat satu profil `sales_users` per nama/kode sales di sumber. Sumber mengeja orang yang
sama beberapa kali ("Granike Monica · Semua cabang", "Granike Monica M. · Semarang"; "Elisa" dua kali), sehingga
pilihan sales di Pusat kendali/Orchestrator penuh duplikat dan dealer seorang sales terpecah.

## Keputusan
- Kolom `sales_users.merged_into` (migrasi 0019). Profil sumber yang digabung tetap ada (kunci impor `source_id`),
  tetapi `active = false` dan tidak muncul di daftar sales mana pun (`ListSalesUsers` hanya `active`, `ListTeam`
  tanpa yang digabung).
- Halaman **Master data → Mapping sales** (`GET /sales-map`, `POST /sales-map/merge {sources, target_id}`), untuk
  pemegang menu Pengguna atau Pengaturan. Menggabung = dalam satu transaksi memindahkan dealer (`owner_id`),
  percakapan, nomor WA, sinyal, interaksi bulanan, login pengguna, dan nomor utama ke sales tujuan; `data_mappings`
  (kind `sales`) untuk kode & nama sumber diarahkan ke tujuan. Keputusan lama (`decided_by`, `confirmed_by`) tetap
  mencatat siapa yang memutuskan.
- Impor berikutnya menghormati gabungan: ejaan yang mapping-nya menunjuk sales lain ditandai `merged_into` lagi, dan
  pencocokan nama memakai sales tujuan.
- Memisah lagi (`target_id: null`) mengaktifkan profil, mengarahkan mapping ke dirinya, lalu memproses ulang data
  impor agar dealernya kembali.
- Sales GSI Orbit yang menjadi tujuan bisa profil baru buatan tangan (Master data → Tim sales), mis. "Granike Monika".

## Konsekuensi
Tidak ada data Odoo/Accurate yang diubah; hanya salinan di GSI Orbit. Menggabung bisa dibatalkan.
