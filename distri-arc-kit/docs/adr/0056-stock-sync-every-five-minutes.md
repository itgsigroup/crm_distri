# ADR 0056 — Stok dari BigQuery tiap 5 menit untuk Push stok

Status: diterima · 2026-10-19 · Sam ("data push stok dari BigQuery pastikan realtime, analisa pakai Claude, sesuai MD")

## Konteks
Push stok membaca stok dari BigQuery lewat sinkron penuh (dealer, faktur, baris faktur, stok) paling cepat tiap 15
menit, dan satu sinkron penuh memakan beberapa menit. Event `data_synced` dikirim server tetapi tidak didengar web,
sehingga halaman tidak menyegarkan diri setelah sinkron. Analisa Stok sudah ditulis Claude lewat MCP (ADR 0030/0049);
komponen halaman sudah sesuai MD (AgingList, Critical, SalesByProduct).

## Keputusan
- Job `stock.sync` (dicek tiap menit): bila BigQuery tidak dibaca dalam `data.source.bigquery.stock_minutes` (bawaan
  5, minimal 2), hanya query **stok** yang dijalankan lalu apply inkremental (faktur tidak dibangun ulang). Berbagi
  kunci data dengan sinkron penuh (tidak berjalan bersamaan). Sinkron penuh tetap tiap 15 menit.
- `GET /api/stock/status` (sumber, waktu sinkron terakhir, interval) dan `POST /api/stock/sync` (pemegang halaman Push
  stok). Halaman menampilkan "Stok dari Accurate (BigQuery) · sinkron HH.MM (n menit lalu) · otomatis tiap 5 menit" dan
  tombol **Sinkron sekarang**; kuning bila terlambat > 3 interval.
- Web mendengar `data_synced`: stok, dealer, orbit, segmen, KPI, kredit, tren, agenda disegarkan setelah sinkron.

## Konsekuensi
- "Real-time" = paling lambat ±5 menit di belakang BigQuery (BigQuery sendiri mengikuti jadwal ekspor Accurate).
- ±288 query stok per hari ke BigQuery (ditagih per byte yang dipindai, minimum 10 MB per query); tabel stok kecil
  sehingga biayanya kecil, tetapi tidak nol — interval bisa dinaikkan lewat `stock_minutes`.
