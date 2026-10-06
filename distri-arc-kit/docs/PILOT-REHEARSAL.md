# Gladi pilot (simulasi) — 2026-10-06

> **Simulasi di database uji, bukan hasil pilot.** Data contoh 18 dealer (fake Odoo, fake WhatsApp), "keputusan
> manusia" dibuat deterministik oleh program. Tujuannya menguji perangkat pilot ujung-ke-ujung sebelum dipakai dengan
> data nyata. Ulangi: `arc ctl pilot rehearse --days 21` pada database kosong yang sudah di-`migrate` + `seed`
> (APP_ENV=dev saja).

## Skenario
- 21 hari mulai Senin 5 Okt 2026: 14 hari **mode bayangan**, lalu **live**. Satu siklus Orchestrator per hari (07.00),
  metrik + snapshot harian, snapshot pilot tiap minggu.
- "Manusia" (CEO) memutuskan ±85% saran tiap hari; sisanya dibiarkan kedaluwarsa. Tingkat tolak per agen: AI Order 5%,
  Follow-up 10%, Penagihan 12%, Prospek 20%, Kredit 30%, Stok 35%.
- 60% follow-up / SO yang disetujui diikuti order dealer dua hari kemudian (seolah dari sinkron Odoo).
- Outbox dikirim tiap sore lewat transport palsu; audit pilot dijalankan tiap hari.

## Hasil (setelah perbaikan di bawah)
| | Mode bayangan (14 hari) | Live (7 hari) |
|---|---|---|
| Saran / disetujui / ditolak | 128 / 96 / 17 | 47 / 34 / 5 |
| Pesan & tulis Odoo terkirim | **0** | 63 (semua dari keputusan tercatat) |
| Audit pilot (6 pemeriksaan) | bersih tiap hari | bersih tiap hari |

- Snapshot mingguan tersimpan untuk 3 minggu; CSV mingguan berjalan.
- Aturan buka otonomi: **AI Penagihan** memenuhi syarat (dua minggu ≥ 80%, ≥ 5 keputusan/minggu) dan dibuka di hari
  ke-15. AI Stok tidak (57% di minggu pertama), AI Follow-up tidak (4 keputusan di minggu pertama), AI Order / Kredit /
  Prospek terlalu sedikit saran di cabang Semarang data contoh. Setelah dibuka pun AI Penagihan tidak membuat langkah
  otomatis, karena matriks hanya mengizinkan "pengingat H-3 ramah" dan semua tagihan di data contoh sudah lewat tempo
  (nada tegas tetap butuh approve) — sesuai kebijakan.

## Masalah yang ditemukan dan diperbaiki
1. **Audit salah alarm** — catatan internal Odoo "Ditolak oleh Sam …" untuk saran yang *ditolak* terhitung sebagai
   "kirim tanpa persetujuan". Catatan itu merekam keputusan dan tidak sampai ke dealer. Audit kini mewajibkan
   persetujuan untuk kanal yang sampai ke dealer / membuat order (`wa`, `odoo_so_draft`) dan keputusan tercatat untuk
   `odoo_note`. Test: `TestPilotAuditViolations`.
2. **Transfer / PO / bundle yang sudah disetujui diusulkan ulang setiap hari** (kunci dedupe memuat tanggal): 20×
   persetujuan untuk transfer yang sama selama gudang menunggu stok bergerak di Odoo. Orchestrator kini tidak
   mengusulkan subjek stok yang sama (SKU + cabang) selama 7 hari setelah disetujui. Saran AI Stok di gladi turun
   dari 214 menjadi 82. Test: `TestApprovedTransferNotReproposed`.

## Catatan untuk pilot nyata
- Dengan 2 sales, beberapa agen (AI Order, AI Kredit, AI Prospek) mungkin tidak mencapai 5 keputusan per minggu,
  sehingga tidak pernah memenuhi syarat buka otonomi dalam pilot 2 minggu (lihat OPEN-QUESTIONS).
- Angka "order tepat jadwal" di gladi turun karena data contoh tidak menerima order baru selain yang disimulasikan;
  di pilot nyata angka ini datang dari sinkron Odoo.
