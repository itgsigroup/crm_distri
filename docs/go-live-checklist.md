# Go-live checklist ARC

Ditandatangani Sam (CEO) sebelum ARC dipakai tim. Isi kolom "Bukti" dengan tautan/foto/perintah.

## Teknis
| # | Item | Bukti | ✓ |
|---|---|---|---|
| 1 | `.env` produksi lengkap; `ARC_ENV=production`, `ARC_CLOCK_ANCHOR` kosong, `ARC_PUBLIC_URL` https | | |
| 2 | `make test` & `make lint` hijau di commit rilis `v1.0.0` | | |
| 3 | Restore backup di mesin bersih berhasil (`scripts/restore.sh`) | | |
| 4 | 48 jam staging tanpa error; biaya AI tercatat (`/api/metrics`, `/api/llm/usage`) | | |
| 5 | Backup harian terjadwal + salinan offsite | | |
| 6 | Disk terenkripsi; port 3001/5432 tidak terbuka ke internet | | |
| 7 | Uji WhatsApp nomor asli lolos (`docs/testing/whatsapp.md`) | | |
| 8 | Playwright end-to-end lolos (`make e2e`) | | |
| 9 | Kredensial Odoo/Google/Anthropic terisi dan sinkron pertama ditinjau | | |

## Bisnis & privasi
| # | Item | Keputusan | ✓ |
|---|---|---|---|
| 10 | **Consent**: karyawan diberi tahu nomor kerja dibaca ARC; chat pribadi antar karyawan tidak dibaca | | |
| 11 | **Policy** (diskon, plafon kredit, ambang sunyi, benchmark L2C) dikonfirmasi di Pengaturan | | |
| 12 | **Target** kuartal ini & berikutnya | | |
| 13 | **Registry nomor internal** lengkap (impor CSV Talenta + konfirmasi saran ARC) | | |
| 14 | **Grup opt-in**: daftar grup eksternal yang boleh dibaca + grup internal (jadwal/tugas saja) | | |
| 15 | **Penerima brief** pagi/sore | | |
| 16 | Sales menerima pelatihan 1 halaman (`docs/training/sales-1-halaman.md`) | | |
| 17 | Semua stage `done-with-mocks` ditinjau; `docs/OPEN-QUESTIONS.md` menjadi keputusan/tiket | | |

Tanda tangan: ____________________ (Sam Setiadi, CEO) · Tanggal: __________
