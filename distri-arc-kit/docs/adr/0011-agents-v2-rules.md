# ADR 0011 — Agen v2: aturan stok kritis, prediksi kas, identifikasi nomor
**Status**: diterima · 2026-10-06 · melengkapi `05-agents.md` (agen 4–6) dan `01-glossary.md`

## Keputusan
1. **Stok kritis (AI Stok)** — SKU dengan sisa < `stock.rules.critical_days`:
   transfer dari cabang lain bila cabang sumber tetap punya ≥ 4 minggu penjualan setelah transfer; jumlah = 2,5 minggu
   penjualan cabang tujuan − sisa, dibulatkan ke 10 terdekat (Semarang kamera 4MP: 18 → +40 dari Jakarta, sama dengan
   mockup). Tanpa sumber → permintaan PO = 5 minggu penjualan − sisa (HDD 4TB: 40 unit). Keduanya `approve`; setelah
   disetujui menjadi `outbox` `odoo_note` (`internal_transfer` / `purchase_request`) yang ditulis ke Odoo di Stage 12.
2. **Prediksi kas masuk 30 hari** = Σ sisa invoice × probabilitas bayar dalam 30 hari:
   `base = min(0,95; 0,42 + 0,55 × tepat waktu)`; pola bayar jatuh di luar 30 hari → `base × 30 / hari menuju pola`;
   sudah lewat tempo → `base × max(0,2; 1 − hari lewat / 60)`; dealer minta tempo (akar "proyek belum cair") → × 0,65.
   Konstanta dikalibrasi ke mockup (Indo 92%, Sinar 90%, Graha 55%, Mitra 45%); seed → Rp 0,99 M (mockup Rp 1,02 M,
   piutang seed Rp 1,25 M vs mockup Rp 1,28 M).
3. **Follow-up dealer yang minta tempo** berjenis `installment` (agen "AI Follow-up + AI Penagihan"): satu usulan berisi
   cicilan 2× + order cash kecil; AI Penagihan memberi `installment` (bukan pengingat ketiga) untuk dealer yang minta
   tempo atau lewat > 14 hari, lalu digabung ke usulan gabungan (aturan `dedupe` covers).
4. **Identifikasi nomor (AI Prospek)** — `internal/identify`: sumber profil WA Business (dibaca worker lewat
   koneksi nomor sales; `wa.ProfileReader`), Truecaller (adapter; fake tanpa API), Getcontact (impor CSV manual), Odoo
   (kontak). Skor: 30 per sumber bernama, +14 bila nama sepakat, Odoo = 100 (sampel Toko Mandiri = 74). Hanya nomor yang
   lebih dulu menulis ke nomor sales (`ErrNotInbound`). Job `identify.number` dipicu ingest untuk nomor baru.
5. **Kirim harga ke nomor baru**: AI Prospek memisah `new_dealer` (catatan Odoo) dan `price_list` (draft WA tier C).
   `price_list` belum punya dealer, jadi outbox mengirim ke thread nomor itu dari nomor sales yang menerima pesan.
6. **Seed menulis sinyal `stock`** dengan kunci yang sama dengan sinkron Odoo (`stock.quant:<id>:<write_date>`), agar
   usulan stok punya provenance pada data contoh tanpa duplikasi saat sinkron berjalan.
