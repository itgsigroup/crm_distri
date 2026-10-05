# Stage 11 — Pengaturan: kebijakan orbit, matriks otonomi, sumber sinyal, pengguna & peran, Panduan
**Baca**: `09-policies-security.md`, `07-api.md` (Pengaturan, Auth), mockup Pengaturan dan Panduan (`reference/panduan-orbit.html`).

## Tujuan
Semua ambang dan aturan diedit dari UI dengan validasi, versi, dan audit; pengguna & peran nyata; Panduan tersedia di dalam aplikasi.

## Deliverables
1. `internal/policy`: JSON Schema per key, `PUT /policies/{key}` dengan validasi + versi + history + audit; cache dengan invalidasi NOTIFY; Orchestrator memuat versi terbaru per siklus.
2. Auth nyata: `users` + login/password (argon2id), sesi JWT HttpOnly, RBAC (ceo/sales/admin/finance/warehouse) di handler dan query; `arc ctl user add`.
3. Frontend Pengaturan persis mockup: Kebijakan orbit (ambang lewat jadwal 1,2×/1,5×, ambang segmen, syarat Key account, limit default per tier, rilis di atas limit, SOP-SEC-001 terkunci, floor margin, follow-up terjadwal, share of wallet), Sumber sinyal (Odoo, WhatsApp, Identifikasi nomor), Koneksi AI (dari Stage 07), Kalibrasi (Stage 10), Pengguna & peran, Matriks otonomi (edit per sel, ceo).
4. Layar **Panduan**: konten `reference/panduan-orbit.html` dipindah ke komponen React (SVG tetap inline), dengan tombol *Buka di aplikasi* per istilah.
5. Uji: validasi schema (nilai di luar batas ditolak), history, RBAC (sales tidak bisa PUT policies; finance boleh decide `collect`).

## Acceptance
- Ubah ambang lewat jadwal ke 1,5× → siklus berikutnya mengubah jumlah At risk (uji e2e dengan seed).
- Login 3 peran berbeda menampilkan menu/aksi sesuai peran.
- Panduan tampil di `/panduan` dan dari tombol *Cara baca*.
- `make check` hijau.

Commit: `feat(stage-11): pengaturan kebijakan, auth & peran, panduan`
