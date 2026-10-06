# ADR 0010 — Peta relasi: riwayat interaksi impor + hitung live, layout di Web Worker
**Status**: diterima · 2026-10-06

## Keputusan
1. **`interactions_monthly`** menyimpan interaksi (WA + order) per (nomor sales, dealer, bulan) yang diimpor sebelum
   Distri ARC (`source=seed` pada data contoh; impor backfill WhatsApp pada pilot) dengan `as_of`. Semua yang terjadi
   setelah `as_of` dihitung live dari `chat_messages` (thread dealer) dan `orders` — tidak ada angka yang dihitung dua kali.
   Data contoh: `as_of` = akhir hari jangkar (5 Okt 2026), sehingga Rizky ↔ Indo Vision tetap 104 seperti mockup.
2. **PIC aktif**: `contacts.interactions_base/base_as_of` menyimpan baseline impor; `dealersvc.Recompute` menghitung ulang
   `interactions_90d` (baseline bila masih di jendela 90 hari + pesan masuk setelahnya) dan `last_interaction_at`
   sebelum metrik. ≤ 1 PIC → AI Follow-up meminta nomor admin/kasir di draft.
3. **Insight relasi** (`views.RelasiInsights`, murni): lewat jadwal (cyc > drift) dengan bulan terakhir di bawah rata-rata
   bulan sebelumnya; dua sales pada satu dealer; rata-rata order ≥ Rp 60 jt dengan < 50 interaksi/bulan.
4. **Frontend**: `three` 0.160 (ADR 0001). Engine `createNet` mockup diport ke `NetView.ts`; settle awal 420 langkah di
   Web Worker dengan PRNG ber-seed (posisi deterministik, diuji vitest), langkah ringan per frame di thread utama
   (22 node). Tanpa WebGL → proyeksi canvas 2D; `prefers-reduced-motion` mematikan rotasi otomatis.
