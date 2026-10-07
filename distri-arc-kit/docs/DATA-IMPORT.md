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

## Operasi
- Sinkron BigQuery otomatis tiap `sync_minutes` (min. 15). Manual: **Sinkron sekarang** / `POST /api/data/sync`.
- Riwayat impor: baris diterima, dilewati (kolom wajib kosong), nilai belum dipetakan, galat.
- Mengosongkan semua data (mis. pindah dari data contoh): backup → `arc ctl wipe --confirm <database>` → `arc ctl user add …`.
