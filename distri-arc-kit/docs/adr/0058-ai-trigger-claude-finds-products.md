# ADR 0058 — AI Trigger: Claude mencari barang di pesan, angka tetap dari sistem

Status: diterima · 2026-10-21 · Sam

## Konteks
Kartu "Tanya stok" mengenali barang dengan pencocokan kata. Uji pada katalog produksi (1.154 baris stok): pesan
"H8c 2mp : 8 @580rb · sekalian nvr ezviz X5S-8W … 1 unit" hanya memunculkan X5S 8W — H8c 2MP (8 unit) terlewat dan
jumlah yang diminta tidak dibandingkan; "stok nvr ezviz 8ch" langsung memilih satu NVR walau tipenya tidak disebut.
Sam: "ingat pakai AI Claude untuk mencari stok".

## Keputusan
- Sistem memilih ≤ 40 kandidat dari katalog stok (`replyrule.Candidates`: kata pesan bobot 3, riwayat bobot 1; kode
  model dan kata SKU bernilai lebih). **Claude** (model mesin AI Trigger bila Claude, selain itu `claude-haiku-5-5`)
  memilih SEMUA barang yang dimaksud beserta jumlah yang diminta (`reply.match`, prompt `reply_match.md`). SKU di luar
  kandidat ditolak; tipe yang tidak jelas tidak ditebak (catatan, tanpa barang).
- Bacaan disimpan sekali per pesan (`reply_ai_matches`, migrasi 0059: item, model, waktu — provenance), dibuat oleh
  job `reply.ai` sebelum draf ditulis; pesan lama dibaca ulang sekali saat chat dibuka.
- Stok per cabang, harga, dan cukup/kurang terhadap jumlah diminta tetap dihitung Go (`SuggestMatched`). Beberapa
  barang = satu kartu dengan baris per barang; draf AI menjawab semuanya (`produk_2`, `jumlah_2`, `diminta`, …).
- Selama Claude membaca pesan baru, kartu menampilkan "Claude mencari barang…" — tidak ada tebakan kata kunci.
  Tanpa kunci Claude atau bila Claude gagal: pencocokan kata, dengan keterangan di kartu.

## Akibat
- Satu panggilan Claude per pesan pelanggan yang memuat kata kunci stok (Haiku, ±1–2 detik, beberapa rupiah).
- Data pelanggan yang dikirim: teks pesan dan riwayat (dimasking oleh router LLM) serta nama barang/stok — sama
  dengan draf AI (ADR 0051).
