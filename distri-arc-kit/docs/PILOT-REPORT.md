# Laporan Pilot — Distri ARC Orbit · Cabang Semarang

> Status: **belum dimulai**. Perangkat pilot sudah siap (Stage 14). Bagian bertanda ✍️ diisi dari data nyata selama dan
> sesudah pilot; angka diambil dari Pengaturan → Pilot / `arc ctl pilot status` / CSV mingguan — bukan diketik ulang
> dari ingatan.

## 1. Persiapan (hari 0)
| # | Langkah | Perintah / layar | Penanggung jawab | Selesai |
|---|---|---|---|---|
| 1 | Deploy produksi (HTTPS, backup, check-env tanpa FAIL) | `docs/DEPLOY.md` · `arc ctl check-env` | IT GSI | ☐ |
| 2 | Kebijakan default + akun CEO, admin, 2 sales Semarang, finance, gudang | `arc ctl seed --policies-only` · `arc ctl user add …` | Sam | ☐ |
| 3 | 2FA aktif untuk CEO & admin | Pengaturan → Keamanan akun | Sam, admin | ☐ |
| 4 | Odoo: user read-only, sinkron penuh cabang Semarang | `arc ctl odoo test` · `arc ctl odoo sync --full` | IT GSI | ☐ |
| 5 | Nomor internal (karyawan) diimpor — DM antar nomor ini tidak disimpan | `arc ctl wa import-internal --csv internal.csv` | admin | ☐ |
| 6 | Pairing WhatsApp 2 nomor sales (QR, linked device) | Pengaturan → WhatsApp | sales + admin | ☐ |
| 7 | Grup WA: gudang/internal ditandai *internal*, grup proyek *eksternal* | Pengaturan → WhatsApp → Grup | admin | ☐ |
| 8 | Konfirmasi share of wallet 20 dealer teratas | Dealer → Konfirmasi share of wallet | 2 sales | ☐ |
| 9 | Mulai mode bayangan | Pengaturan → Pilot → *Bayangan* (atau `arc ctl pilot start --branch Semarang`) | Sam | ☐ |

## 2. Aturan selama pilot
- **Minggu 1–2: mode bayangan.** Orchestrator berjalan tiap jam 06.00–20.00; semua saran butuh approve; *tidak ada yang
  dikirim* ke dealer maupun ditulis ke Odoo (outbox berstatus `shadow`). Sales tetap membalas dari ponsel. Sam & sales
  menilai saran (setujui / edit / tolak dengan alasan) — itu yang mengkalibrasi agen.
- **Setelah 14 hari: mode live** (Sam memutuskan). Kirim hanya setelah disetujui manusia. Langkah otomatis per agen
  dibuka satu per satu di Pengaturan → Pilot → *Buka otonomi*, hanya bila agen ≥ 80% diterima dua minggu penuh
  berturut-turut dengan ≥ 5 keputusan per minggu (dicek server).
- **Audit harian** (`arc ctl pilot audit`, exit 1 bila ada pelanggaran) dan kartu Audit di Pengaturan → Pilot:
  kirim tanpa keputusan tercatat, kirim selama mode bayangan, alert ke selain grup internal, DM antar nomor internal
  tersimpan, pesan grup internal masuk ke dealer, identifikasi nomor yang tidak menghubungi lebih dulu. Semua harus 0.
- **Snapshot mingguan** otomatis tiap Senin 00.45 WIB; CSV per minggu dari Pengaturan → Pilot atau
  `arc ctl pilot export --week YYYY-MM-DD --out minggu.csv`.

## 3. Hasil ✍️
| Indikator | Baseline (hari 0) | Minggu 1 | Minggu 2 | Target |
|---|---|---|---|---|
| Saran diterima (semua agen) | — | | | ≥ 70% |
| Median waktu keputusan | — | | | < 4 jam kerja |
| Order tepat jadwal | | | | 85% |
| DSO (hari) | | | | 30 |
| Lewat jadwal tertangkap sebelum churn | — | | | ≥ 50% |
| Pelanggaran audit | — | 0 | 0 | 0 |
| Kirim selama mode bayangan | — | 0 | 0 | 0 |

Per agen (dari Pengaturan → Pilot):
| Agen | Saran | Diterima | Median keputusan | 2 minggu ≥ 80%? | Otonomi dibuka? |
|---|---|---|---|---|---|
| AI Order | | | | | |
| AI Follow-up | | | | | |
| AI Kredit | | | | | (tidak pernah otonom untuk rilis) |
| AI Stok | | | | | |
| AI Penagihan | | | | | |
| AI Prospek | | | | | |

## 4. Temuan ✍️
- Alasan penolakan terbanyak per agen (Pengaturan → Kalibrasi agen · pelajaran):
- Istilah yang membingungkan tim (perlu diubah di glossary / Panduan?):
- Masalah teknis (RUNBOOK: WA terputus, Odoo sync, LLM):

## 5. Perubahan kebijakan selama pilot ✍️
| Tanggal | Kebijakan | Dari → ke | Alasan | Oleh |
|---|---|---|---|---|
| | | | | |
(riwayat lengkap: `policy_history` / Pengaturan → editor kebijakan → Riwayat)

## 6. Rekomendasi cabang berikutnya ✍️
- Cabang:
- Agen yang langsung dibuka otonominya di cabang berikutnya:
- Yang harus diperbaiki dulu:

## 7. Keputusan
☐ **Go** — lanjut cabang kedua: ______________________ mulai ____________
☐ **No-go** — alasan: ______________________________________________

Ditandatangani: ______________________ (Sam Setiadi, CEO GSI) · tanggal ____________
