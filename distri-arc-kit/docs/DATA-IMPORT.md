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

## Menghubungkan BigQuery (data Accurate)
1. Google Cloud console → project (mis. `empyrean-surge-505203-u5`) → **IAM & Admin → Service Accounts → Create**
   (mis. `distri-arc-reader`), peran **BigQuery Data Viewer** + **BigQuery Job User**.
2. Service account itu → **Keys → Add key → JSON**. Unggah file di Pengaturan → Data & master → Sumber data asli →
   BigQuery → **Unggah JSON** (hanya CEO; disimpan terenkripsi dengan `SESSION_SECRET`). Jangan kirim kunci lewat chat/email.
3. Isi **Project ID** (dan Lokasi bila dataset di luar US, mis. `asia-southeast2`), Simpan.
4. Kartu **Pemetaan BigQuery** → **Jelajahi BigQuery**: Distri ARC membaca daftar tabel dan kolom (baca saja), lalu
   menyarankan tabel dan kolom untuk tiap jenis data. Periksa per tab (Tim sales, Pelanggan, Faktur, Item faktur, Stok),
   betulkan kolom yang salah (boleh ekspresi, mis. `IFNULL(sisa, 0)`), tambah filter bila perlu, **Simpan query**.
5. **Tes query** (5 baris contoh per query) → **Sinkron sekarang**.
6. Mapping master: nilai kategori dan jenis pelanggan yang muncul diberi **saran** (mis. "IP Camera" → Kamera & NVR,
   "System Integrator" → Freelance / SI). **Isi saran** → periksa → **Simpan**. Cabang, gudang, dan sales dipetakan manual.

## Kontrak kolom (* wajib)
| Data | Kolom |
|---|---|
| `sales` | code*, name*, branch, wa_number, email, title |
| `customers` | code*, name*, customer_type, branch, city, sales, phone, pic_name, tier, credit_limit, payment_terms_days, segment |
| `invoices` | number*, customer_code*, date*, due_date, total*, residual, paid_date, branch, sales |
| `invoice_lines` | invoice_number*, product*, sku, category, brand, qty, price, amount, cost, warehouse |
| `stock` | sku*, product*, category, warehouse*, qty*, unit_cost, value, age_days, received_date, sold_90d |

Angka boleh format Indonesia (`1.234.567,50`) atau internasional; tanggal `YYYY-MM-DD`, `DD/MM/YYYY`, atau timestamp.
`residual` = sisa tagihan (0 = lunas); tanpa `residual` dan tanpa `paid_date`, faktur dianggap belum dibayar.

## Halaman Push stok dari BigQuery
Setiap angka di halaman Push stok dihitung dari rumus `01-glossary.md` atas data impor:

| Bagian halaman | Rumus (glossary) | Kolom BigQuery yang dipakai |
|---|---|---|
| **Perputaran stok** (target ≤ 40 hari) | nilai stok ÷ HPP harian 90 hari | `stock.value` (atau `qty × unit_cost`); HPP penjualan dari `invoice_lines.cost` — bila kosong, `stock.unit_cost` per SKU |
| **Stok > 90 hari** & **Push stok** | stok menua `age_days > policy.aging_days (90)`; kandidat = product mix cocok ∧ jadwal order ≤ 7 hari / lewat jadwal ∧ limit tidak over/overdue | `stock.age_days`, atau dihitung dari `stock.received_date` (tanggal masuk / pembelian terakhir); `stock.category` → 6 kategori lewat mapping; kandidat dari pelanggan & faktur |
| **Stok kritis** (habis < 10 hari) | `qty ÷ (terjual 90 hari ÷ 13) × 7` | `stock.qty`; `stock.sold_90d`, atau dihitung dari `invoice_lines.qty` 90 hari terakhir per SKU × cabang (gudang item faktur, else cabang faktur/pelanggan) |
| **Penjualan per produk · 30 hari** (nilai · margin) | Σ nilai baris; margin = 1 − HPP ÷ nilai | `invoice_lines.amount`, `qty`, `cost` (atau HPP stok) |

Gudang dipetakan ke cabang (Pengaturan → Data & master → Mapping → Gudang) — stok dijumlah per SKU × cabang.

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
       kuantitas AS qty, harga_satuan AS price, nilai_penjualan AS amount, hpp AS cost, gudang AS warehouse
FROM `gsi-data.accurate.faktur_item` WHERE tanggal_faktur >= DATE_SUB(CURRENT_DATE(), INTERVAL 12 MONTH)

-- stock (snapshot per gudang hari ini). Tanpa kolom umur/terjual: kirim tanggal masuk terakhir saja —
-- umur dihitung dari received_date dan kecepatan jual dari item faktur 90 hari.
SELECT kode_item AS sku, nama_item AS product, kategori AS category, gudang AS warehouse, kuantitas AS qty,
       harga_pokok AS unit_cost, kuantitas * harga_pokok AS value, tanggal_masuk_terakhir AS received_date
FROM `gsi-data.accurate.stok_item` WHERE kuantitas <> 0
```
**Kunci**: Google Cloud → IAM → Service accounts → buat akun, peran **BigQuery Data Viewer** + **BigQuery Job User**
→ Keys → JSON. Unggah di kartu Sumber data (CEO); kunci disimpan terenkripsi dengan `SESSION_SECRET`.
Tombol **Tes query** menjalankan tiap query dengan `LIMIT 5` dan memeriksa kolom wajib.

## Data Accurate GSI (BigQuery `empyrean-surge-505203-u5`, dataset `accurate_data`, lokasi `asia-southeast2`)
Terpasang 2026-10-07 dengan service account `distri-data@…`, sinkron tiap 60 menit. Pemetaan:

| Distri ARC | Tabel Accurate | Catatan |
|---|---|---|
| Tim sales | `sales_report` (18 bln) + `tipe_sales` | cabang = cabang penjualan terbanyak; jabatan = jenis_sales (Distri/Project/AE/…) |
| Pelanggan | `master_pelanggan` | hanya B2B aktif: Dealer, VIP Dealer, Kanvas Dalam/Luar Kota → **reseller**; SI, Freelance, Installer, Kontraktor → **si**. Umum/User/ONLINE (konsumen akhir, marketplace) tidak masuk Orbit. Sales pemegang = sales terbanyak 18 bln |
| Faktur | `faktur_penjualan_per_penjual` (18 bln) | `piutang` = sisa tagihan; kode pelanggan = `id_pelanggan` (= `kode_pelanggan`) |
| Item faktur | `sales_report` (18 bln) | nilai **tanpa PPN** (`harga_penjualan_tanpa_ppn`), HPP = `hpp_satuan` → margin per produk |
| Stok | `stok_gudang_gsi` snapshot terakhir | per barang × cabang, tanpa gudang Display/Servis/Rusak/Demo/Pesanan/Transit/Konsinyasi/Dropship; umur = `tanggal_masuk` lot tertua di `master_aging`; terjual 90 hari dari `sales_report`; HPP cadangan `perhitungan_biaya_satuan` → `hpp_item` |

Cabang (master Cabang): Kantor Pusat → **Semarang** (gudang Lamper), Cabang Yogya → Yogyakarta, Cabang Jakarta → Jakarta,
Cabang Surabaya → Surabaya, Mataram → Mataram, Soetta → Soetta (gudang). 133 kategori barang Accurate dipetakan ke 6
kategori (Jasa dan Solution sengaja tidak — bukan produk). Ubah di Pengaturan → Data & master → Mapping.

<details><summary>Query yang terpasang</summary>


`sales`
```sql
WITH s AS (
  SELECT nama_sales, nama_cabang FROM `empyrean-surge-505203-u5.accurate_data.sales_report`
  WHERE tanggal_invoice >= DATE_SUB(CURRENT_DATE(), INTERVAL 18 MONTH) AND TRIM(IFNULL(nama_sales, '')) <> ''
)
SELECT nama_sales AS code, nama_sales AS name,
       APPROX_TOP_COUNT(nama_cabang, 1)[OFFSET(0)].value AS branch,
       (SELECT ANY_VALUE(t.jenis_sales) FROM `empyrean-surge-505203-u5.accurate_data.tipe_sales` t WHERE t.nama_sales = s.nama_sales) AS title
FROM s GROUP BY nama_sales
```

`customers`
```sql
WITH a AS (
  SELECT kode_pelanggan,
         APPROX_TOP_COUNT(NULLIF(TRIM(nama_sales), ''), 1)[OFFSET(0)].value AS sales,
         APPROX_TOP_COUNT(nama_cabang, 1)[OFFSET(0)].value AS cabang
  FROM `empyrean-surge-505203-u5.accurate_data.sales_report` WHERE tanggal_invoice >= DATE_SUB(CURRENT_DATE(), INTERVAL 18 MONTH)
  GROUP BY kode_pelanggan
)
SELECT p.kode_pelanggan AS code, p.nama_pelanggan AS name, p.kategori_pelanggan AS customer_type,
       COALESCE(NULLIF(p.cabang, '[Semua Cabang]'), a.cabang) AS branch, p.kota_pelanggan AS city, a.sales AS sales,
       COALESCE(NULLIF(TRIM(p.telepon_pelanggan), ''), p.telepon_kantor_pelanggan) AS phone, p.kontak_pelanggan AS pic_name,
       CAST(ROUND(IF(p.pakai_limit_nilai, IFNULL(p.limit_nilai, 0), 0)) AS INT64) AS credit_limit,
       IFNULL(SAFE_CAST(REGEXP_EXTRACT(LOWER(p.termin_pembayaran), r'(\d+)') AS INT64), 0) AS payment_terms_days,
       p.kategori_pelanggan AS segment
FROM `empyrean-surge-505203-u5.accurate_data.master_pelanggan` p LEFT JOIN a USING (kode_pelanggan)
WHERE NOT IFNULL(p.nonaktif, FALSE) AND p.kategori_pelanggan IN ('Dealer','VIP Dealer','Kanvas Dalam Kota','Kanvas Luar Kota','SI','Freelance','Installer','Kontraktor')
```

`invoices`
```sql
SELECT f.no_invoice AS number, f.id_pelanggan AS customer_code, f.tanggal_invoice AS date, f.jatuh_tempo AS due_date,
       CAST(ROUND(f.total) AS INT64) AS total, CAST(ROUND(GREATEST(IFNULL(f.piutang, 0), 0)) AS INT64) AS residual,
       f.tanggal_terakhir_bayar AS paid_date, f.nama_cabang AS branch, f.tenaga_penjual AS sales
FROM `empyrean-surge-505203-u5.accurate_data.faktur_penjualan_per_penjual` f
JOIN `empyrean-surge-505203-u5.accurate_data.master_pelanggan` p ON p.kode_pelanggan = f.id_pelanggan
WHERE f.tanggal_invoice >= DATE_SUB(CURRENT_DATE(), INTERVAL 18 MONTH)
  AND NOT IFNULL(p.nonaktif, FALSE) AND p.kategori_pelanggan IN ('Dealer','VIP Dealer','Kanvas Dalam Kota','Kanvas Luar Kota','SI','Freelance','Installer','Kontraktor')
```

`invoice_lines`
```sql
SELECT s.no_invoice AS invoice_number, s.nama_barang AS product, s.kode_barang AS sku, s.kategori_barang AS category,
       s.brand AS brand, FORMAT('%.2f', s.kuantitas_penjualan) AS qty,
       CAST(ROUND(IFNULL(SAFE_DIVIDE(s.harga_penjualan_tanpa_ppn, NULLIF(s.kuantitas_penjualan, 0)), 0)) AS INT64) AS price,
       CAST(ROUND(IFNULL(s.harga_penjualan_tanpa_ppn, 0)) AS INT64) AS amount,
       CAST(ROUND(IFNULL(s.hpp_satuan, 0)) AS INT64) AS cost, s.nama_cabang AS warehouse
FROM `empyrean-surge-505203-u5.accurate_data.sales_report` s
JOIN `empyrean-surge-505203-u5.accurate_data.master_pelanggan` p USING (kode_pelanggan)
WHERE s.tanggal_invoice >= DATE_SUB(CURRENT_DATE(), INTERVAL 18 MONTH)
  AND NOT IFNULL(p.nonaktif, FALSE) AND p.kategori_pelanggan IN ('Dealer','VIP Dealer','Kanvas Dalam Kota','Kanvas Luar Kota','SI','Freelance','Installer','Kontraktor')
ORDER BY s.no_invoice, s.kode_barang, s.kuantitas_penjualan, s.harga_penjualan_tanpa_ppn
```

`stock`
```sql
-- Stok: snapshot terakhir per barang x cabang (gudang jual saja); umur dari lot tertua (master_aging); terjual 90 hari dari sales_report
WITH snap AS (SELECT MAX(tanggal_snapshot) AS d FROM `empyrean-surge-505203-u5.accurate_data.stok_gudang_gsi`),
st AS (
  SELECT g.kode_barang, ANY_VALUE(g.nama_barang) AS nama_barang, ANY_VALUE(g.kategori_barang) AS kategori_barang, g.nama_cabang,
         SUM(g.kuantitas_persediaan) AS qty, SUM(IFNULL(g.rupiah_total_stok, 0)) AS nilai, MAX(NULLIF(g.perhitungan_biaya_satuan, 0)) AS biaya
  FROM `empyrean-surge-505203-u5.accurate_data.stok_gudang_gsi` g, snap
  WHERE g.tanggal_snapshot = snap.d AND g.nama_cabang IN ('Kantor Pusat','Cabang Yogya','Cabang Jakarta','Cabang Surabaya','Mataram','Soetta') AND g.kuantitas_persediaan > 0
    AND NOT REGEXP_CONTAINS(LOWER(g.nama_gudang), r'display|servis|rusak|demo|pesanan|transit|konsinyasi|dropship')
  GROUP BY g.kode_barang, g.nama_cabang
),
hpp AS (
  SELECT kode_barang, ARRAY_AGG(SAFE_DIVIDE(hpp, qty) IGNORE NULLS ORDER BY tanggal DESC LIMIT 1)[SAFE_OFFSET(0)] AS hpp
  FROM `empyrean-surge-505203-u5.accurate_data.hpp_item` WHERE hpp > 0 AND qty > 0 GROUP BY kode_barang
),
age AS (
  SELECT kode_barang, cabang, MIN(tanggal_masuk) AS masuk FROM `empyrean-surge-505203-u5.accurate_data.master_aging`
  WHERE sisa_qty > 0 AND tanggal_masuk IS NOT NULL AND NOT REGEXP_CONTAINS(LOWER(gudang), r'display|servis|rusak|demo|pesanan|transit|konsinyasi|dropship')
  GROUP BY kode_barang, cabang
),
sold AS (
  SELECT kode_barang, nama_cabang, SUM(kuantitas_penjualan) AS q FROM `empyrean-surge-505203-u5.accurate_data.sales_report`
  WHERE tanggal_invoice >= DATE_SUB(CURRENT_DATE(), INTERVAL 90 DAY) GROUP BY kode_barang, nama_cabang
),
x AS (
  SELECT st.*, COALESCE(NULLIF(SAFE_DIVIDE(st.nilai, st.qty), 0), st.biaya, hpp.hpp, 0) AS unit
  FROM st LEFT JOIN hpp USING (kode_barang)
)
SELECT x.kode_barang AS sku, x.nama_barang AS product, x.kategori_barang AS category, x.nama_cabang AS warehouse,
       FORMAT('%.2f', x.qty) AS qty, CAST(ROUND(x.unit) AS INT64) AS unit_cost,
       CAST(ROUND(IF(x.nilai > 0, x.nilai, x.qty * x.unit)) AS INT64) AS value,
       age.masuk AS received_date, FORMAT('%.2f', IFNULL(sold.q, 0)) AS sold_90d
FROM x
LEFT JOIN age ON age.kode_barang = x.kode_barang AND age.cabang = x.nama_cabang
LEFT JOIN sold ON sold.kode_barang = x.kode_barang AND sold.nama_cabang = x.nama_cabang
```

</details>

## Operasi
- Sinkron BigQuery otomatis tiap `sync_minutes` (min. 15). Manual: **Sinkron sekarang** / `POST /api/data/sync`.
- Riwayat impor: baris diterima, dilewati (kolom wajib kosong), nilai belum dipetakan, galat.
- Mengosongkan semua data (mis. pindah dari data contoh): backup → `arc ctl wipe --confirm <database>` → `arc ctl user add …`.
