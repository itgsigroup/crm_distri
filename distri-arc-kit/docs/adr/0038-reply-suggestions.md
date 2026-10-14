# ADR 0038 — Rekomendasi balasan dari data (aturan kata kunci)

Status: diterima · 2026-10-14 (Sam)

## Keputusan
- Tabel `reply_rules` (migrasi 0036): nama, jenis (`stock`), kata kunci, draf bila ready / kosong (placeholder
  `{produk} {jumlah} {cabang} {cabang_lain}`), sebut jumlah atau tidak, aktif. Diatur CEO/admin di Pengaturan →
  Rekomendasi balasan, dengan kotak "Coba kalimat pelanggan". Aturan bawaan "Tanya stok".
- `internal/replyrule` (Go murni): kata kunci dicocokkan sebagai kata utuh; produk dikenali dari kata yang sama dengan
  nama stok (kode model/SKU berbobot dua, satu kata saja tidak cukup); beberapa produk setara → "beberapa produk cocok".
  Stok cabang chat (cabang dealer, else cabang sales), cabang lain yang ada stoknya, dan draf jawaban.
- Dihitung saat chat dibuka, untuk pesan pelanggan 7 hari terakhir (maks 30), bukan disimpan. Tampil di bawah bubble
  dengan jam sinkron stok; "Pakai jawaban" mengisi kolom ketik (sebagai balasan pesan itu). **Tidak pernah dikirim
  otomatis** — sales mengedit dan menekan kirim sendiri.
- Ini fakta dari data, bukan analisa AI: tidak bertentangan dengan ADR 0032.

## Konsekuensi
- Kualitas mengikuti data stok Accurate/BigQuery (laporan direktur, Tahap 0). Harga per tingkat dealer belum ada,
  jadi aturan harga menyusul.
