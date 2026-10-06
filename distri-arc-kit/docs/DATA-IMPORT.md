# Data asli & master data

Distri ARC tidak lagi memakai data contoh di server. Data asli masuk lewat **Pengaturan → Data & master**, dari
**BigQuery** (ditarik otomatis) atau **CSV** (unggah manual), dalam satu format baku per jenis data. Data mentah
disimpan dulu (staging), lalu diproses dengan **mapping master data** menjadi tim sales, pelanggan (dealer), kontak,
order, faktur, pembayaran, stok, katalog produk, dan sinyal provenance untuk agen. Mengubah mapping memproses ulang
semua data tanpa impor ulang.

## Urutan setup
1. **Tim sales** (CSV `sales` / query `sales`, atau tambah manual di kartu Tim sales).
2. **Pelanggan** — dua jenis: **Dealer (reseller)** dan **Freelance / System Integrator** (`customer_type`: `reseller` / `si`;
   ejaan lain seperti "Dealer", "Freelance", "System Integrator" dikenali otomatis, sisanya dipetakan).
3. **Faktur** dan **item faktur** (siklus order, product mix, piutang, pola bayar dihitung dari sini).
4. **Stok per gudang** (snapshot; gudang dipetakan ke cabang dan dijumlah per cabang).
5. **Mapping**: Cabang, Gudang → cabang, Kategori → 6 kategori product mix (Kamera & NVR, HDD & storage, Kabel & PoE,
   Modul LED, Fire alarm, Aksesoris), Sales, Jenis pelanggan. Nilai yang belum dipetakan tampil paling atas (kuning).
6. **Master pelanggan**: tier, limit kredit, termin, sales pemegang, jenis — isian di sini **tidak tertimpa** impor berikutnya.
7. Pengguna login (Pengaturan → Pengguna & peran) dan nomor WhatsApp (Pengaturan → WhatsApp).

## Kontrak kolom (* wajib)
| Data | Kolom |
|---|---|
| `sales` | code*, name*, branch, wa_number, email, title |
| `customers` | code*, name*, customer_type, branch, city, sales, phone, pic_name, tier, credit_limit, payment_terms_days, segment |
| `invoices` | number*, customer_code*, date*, due_date, total*, residual, paid_date, branch, sales |
| `invoice_lines` | invoice_number*, product*, sku, category, brand, qty, price, amount, warehouse |
| `stock` | sku*, product*, category, warehouse*, qty*, unit_cost, value, age_days, sold_90d |

Angka boleh format Indonesia (`1.234.567,50`) atau internasional; tanggal `YYYY-MM-DD`, `DD/MM/YYYY`, atau timestamp.
`residual` = sisa tagihan (0 = lunas); tanpa `residual` dan tanpa `paid_date`, faktur dianggap belum dibayar.

## Contoh query BigQuery
Setiap query cukup memberi nama kolom sesuai kontrak (`AS …`). Contoh untuk tabel hasil ekspor Accurate:
```sql
-- customers
SELECT CAST(kode_pelanggan AS STRING) AS code, nama_pelanggan AS name, tipe_pelanggan AS customer_type,
       cabang AS branch, kota AS city, nama_sales AS sales, telepon AS phone, nama_pic AS pic_name
FROM `gsi-data.accurate.pelanggan`

-- invoices (12 bulan terakhir; sinkron berikutnya memperbarui yang berubah)
SELECT nomor_faktur AS number, CAST(kode_pelanggan AS STRING) AS customer_code, tanggal_faktur AS date,
       tanggal_jatuh_tempo AS due_date, total_faktur AS total, sisa_tagihan AS residual, tanggal_bayar AS paid_date,
       cabang AS branch, sales_user AS sales
FROM `gsi-data.accurate.faktur` WHERE tanggal_faktur >= DATE_SUB(CURRENT_DATE(), INTERVAL 12 MONTH)

-- invoice_lines
SELECT nomor_faktur AS invoice_number, kode_item AS sku, nama_item AS product, kategori AS category, merek AS brand,
       kuantitas AS qty, harga_satuan AS price, nilai_penjualan AS amount, gudang AS warehouse
FROM `gsi-data.accurate.faktur_item` WHERE tanggal_faktur >= DATE_SUB(CURRENT_DATE(), INTERVAL 12 MONTH)

-- stock
SELECT kode_item AS sku, nama_item AS product, kategori AS category, gudang AS warehouse, kuantitas AS qty,
       harga_pokok AS unit_cost, nilai_stok AS value, umur_hari AS age_days, terjual_90_hari AS sold_90d
FROM `gsi-data.accurate.stok_item`
```
**Kunci**: Google Cloud → IAM → Service accounts → buat akun, peran **BigQuery Data Viewer** + **BigQuery Job User**
→ Keys → JSON. Unggah di kartu Sumber data (CEO); kunci disimpan terenkripsi dengan `SESSION_SECRET`.
Tombol **Tes query** menjalankan tiap query dengan `LIMIT 5` dan memeriksa kolom wajib.

## Operasi
- Sinkron BigQuery otomatis tiap `sync_minutes` (min. 15). Manual: **Sinkron sekarang** / `POST /api/data/sync`.
- Riwayat impor: baris diterima, dilewati (kolom wajib kosong), nilai belum dipetakan, galat.
- Mengosongkan semua data (mis. pindah dari data contoh): backup → `arc ctl wipe --confirm <database>` → `arc ctl user add …`.
