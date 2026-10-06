# ADR 0018 — Data asli lewat impor (BigQuery / CSV), staging, dan mapping master data
**Status**: diterima · 2026-10-06 · atas permintaan pemilik. Melengkapi (bukan menggantikan) ADR 0002 sinkron Odoo.

## Konteks
Data penjualan GSI tersebar: faktur, piutang, dan stok ada di **Accurate**; Odoo dipakai untuk CRM dan pelanggan.
Pemilik akan menyediakan data lewat **BigQuery**. Pelanggan GSI ada dua jenis: **Dealer (reseller)** dan **Freelance /
System Integrator** (memasang langsung ke customernya). Nama cabang, gudang, kategori, dan sales di sumber tidak sama
dengan konsep Distri ARC (cabang, 6 kategori product mix, tim sales).

## Keputusan
1. **Kontrak kolom baku** per entitas (`sales`, `customers`, `invoices`, `invoice_lines`, `stock`) — BigQuery (SQL per
   entitas, diatur CEO di Pengaturan) dan CSV memakai kontrak yang sama (`internal/importer`).
2. **Staging lalu transformasi**: baris mentah disimpan di `import_rows` (kunci sumber), lalu diproses menjadi
   tabel Distri ARC (`source_system = 'import'`, idempoten). Perubahan mapping menjalankan ulang transformasi penuh;
   impor biasa hanya faktur yang berubah. Satu transformasi pada satu waktu (advisory lock).
3. **Mapping master data** (`data_mappings`): cabang, gudang → cabang, kategori → 6 KAT, sales → tim, jenis pelanggan.
   Nilai baru tercatat otomatis sebagai "belum dipetakan"; nama sales yang sama dengan anggota tim dipetakan otomatis.
4. **Master pelanggan**: jenis, tier, limit, termin, pemilik diedit di Pengaturan dan dikunci (`master_locked`) agar
   tidak tertimpa impor. Tanpa limit kredit, pelanggan dihitung **cash** sampai limit diisi (tidak ditebak).
5. **Faktur = order** untuk metrik (siklus order, product mix dari item), plus faktur, pembayaran (dari `paid_date`), dan
   sinyal `so` / `invoice` / `payment` / `stock` sebagai provenance agen. Stok adalah snapshot per SKU × cabang.
6. **BigQuery** lewat REST API resmi (jobs.query + getQueryResults) dengan service account (JWT RS256, stdlib), scope
   baca saja; kunci disegel AES-GCM (`secrets`). Tidak ada dependensi Google SDK.
7. **Jenis pelanggan** di `dealers.customer_type`, filter `?type=reseller|si` di board (Dealer, Orbit, Segmen), badge di
   halaman dealer.
8. `arc ctl wipe --confirm <db>` mengosongkan instalasi (data contoh → data asli) dengan konfirmasi nama database.
   Setiap akun baru otomatis mendapat profil (`sales_users`) agar keputusannya tercatat.

## Konsekuensi
- Query BigQuery ditulis sesuai skema dataset GSI (contoh di `docs/DATA-IMPORT.md`); kolom diuji dengan **Tes query**.
- Margin per order belum tersedia (HPP per faktur tidak ada di kontrak) — AI Order memakai harga katalog.
- Odoo sync tetap bisa dipakai bila kelak faktur/stok pindah ke Odoo; keduanya menulis tabel yang sama dengan
  `source_system` berbeda.
