# ADR 0023 — Klasifikasi ulang Orbit & Segmen untuk data Accurate

Status: diterima · 2026-10-08 · mengubah rumus di `01-glossary.md` (Satuan waktu, Status, Segmen)

## Konteks
Dengan data asli (2.841 pelanggan, 18 bulan faktur Accurate) klasifikasi glossary menyimpang:
- 1.995 "Baru": 1.383 belum pernah order, 612 order sekali ±11 bulan lalu.
- Accurate menulis satu faktur per pengiriman (sering beberapa per hari) → siklus order terbaca 1 hari → dealer yang
  order tiap 2–3 bulan menjadi Churn (340 dealer Segmen B Churn).
- Omzet/bln = besarnya × seringnya memproyeksikan frekuensi yang membengkak: Rp42,7 M/bln vs penjualan aktual ±Rp13 M.
- Key account mensyaratkan SOW ≥ 50 sementara SOW default 50 → 47 dealer kecil (Segmen D) menjadi Key account.

## Keputusan
- **Hari order**: faktur di tanggal yang sama = satu order (siklus, besarnya, seringnya).
- **Siklus order** minimal `orbit.min_rhythm_days` (default 7).
- **Seringnya & omzet/bln aktual**: hari order / penjualan 6 bulan dibagi bulan aktif (≤ 6, sejak order pertama).
- **Status baru "Prospek"** untuk pelanggan yang belum pernah order: tidak digambar di Orbit/Segmen, dihitung terpisah
  (+ filter di halaman Dealer). Order sekali > `orbit.new_days` (90) lalu, atau diam > 365 hari → Churn.
- **Churn** juga butuh `orbit.churn_min_days` hari tanpa order (produksi 60): pembeli 2×/minggu yang diam 3 minggu
  masih At risk, belum Churn.
- **Key account** juga butuh `orbit.key_account.omzet_min` (Rp/bln; 0 = tanpa syarat, produksi diset Rp25 jt).
- Ambang Segmen produksi dikalibrasi ke distribusi aktual lewat kebijakan (bukan kode): sering ≥ 1×/bln (±P70),
  besar ≥ Rp10 jt per order (±P75). Tepat waktu Key account 60% (median pembayaran dealer besar 60%, tidak ada yang
  ≥ 85%).

## Akibat
- Seed 18 dealer mockup tetap sama (regresi hijau); default kode tidak berubah kecuali `min_rhythm_days`/`new_days`.
- Angka Orbit/Segmen/KPI di produksi berubah setelah recompute; riwayat harian lama tetap memakai rumus lama.
