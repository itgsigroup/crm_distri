# ADR 0036 — Share of wallet, omzet, dan margin (bug laporan direktur 14 Okt, tugas 11–13)

Status: diterima · 2026-10-14 (Sam)

## Konteks
- Share of wallet 50 untuk semua dealer: estimasi kompetitor dari WA tidak pernah terisi, sehingga semua dealer tanpa
  konfirmasi sales memakai bawaan 50 — dan 50 memenuhi syarat Key account (`sow ≥ 50`).
- Omzet CRM ≠ Sales Arena: kartu dashboard "Omzet dealer · per bulan" menjumlah rata-rata 6 bulan per dealer, bukan
  omzet bulan itu; PPN, faktur batal, dan retur tidak ditangani; sumber dan jam sinkron tidak tampil.
- Margin Ezviz minus: HPP cadangan dari stok diambil per SKU ("baris gudang terakhir menang"), HPP total-baris dibaca
  sebagai HPP satuan, rebate tidak ada.

## Keputusan
- **SOW**: urutan sumber = konfirmasi sales → estimasi kompetitor (bila ada) → **riwayat belanja** (`history`: omzet
  3 bulan terakhir ÷ 3 bulan berturut-turut terbaik dalam 12 bulan × 100, perlu riwayat ≥ 6 bulan) → tidak diketahui
  (`default`, nilai 50 hanya untuk skor; tampil "—" / "belum diketahui"; tidak pernah membuat Key account).
- **Omzet**: kartu dashboard = omzet aktual bulan lalu (+ bulan berjalan s.d. hari ini) dari faktur, rata-rata dealer
  ditampilkan sebagai keterangan. KPI (`/api/kpi`, MCP `kpi.utama`, `penjualan.bulanan`, `margin.cek`) membawa `data`:
  sumber, waktu sinkron terakhir, dan dasar omzet. Kontrak impor faktur menerima `dpp` (omzet tanpa PPN; piutang tetap
  total), `status` (batal/void/cancel → tidak dihitung, piutang 0), `doc_type` (retur → omzet & baris minus, tanpa
  piutang).
- **Margin**: HPP stok cadangan per SKU × cabang, rata-rata tertimbang qty (lalu rata-rata semua cabang); HPP faktur
  yang jelas total baris (> 1,5× harga, dan ÷ qty masuk akal) dibaca per satuan; merek disimpan di baris; kebijakan
  `margin.floor.rebates` {merek: %} mengurangi HPP. Baris menyimpan `cost_source` (invoice | stock). Tool MCP
  `margin.cek` menampilkan per produk: harga rata-rata, HPP dipakai, porsi HPP dari stok, HPP stok, rebate, catatan.

## Konsekuensi
- Status dealer bisa berubah: dealer tanpa konfirmasi SOW dengan riwayat < 6 bulan tidak lagi Key account; dealer yang
  belanjanya turun mendapat SOW rendah.
- DPP/status/retur/merek hanya berlaku bila query BigQuery mengirim kolom itu (OPEN-QUESTIONS).
