# ADR 0041 — AI Trigger: halaman sendiri, empat jenis data, draf boleh ditulis AI

Status: diterima · 2026-10-16 (Sam) · memperluas ADR 0038 (Rekomendasi balasan); pengecualian sempit atas ADR 0032

## Konteks
Sam: "tambahkan 1 halaman untuk trigger AI dan bisa di-custom — ketika customer menulis kalimat tertentu seperti
*stok*, di chat itu tampil hasil AI yang mengambil data stok." ADR 0038 sudah membuat kartu stok dari aturan kata
kunci, tetapi aturannya hanya kartu di Pengaturan dan hanya untuk stok.

## Keputusan
- **Halaman sendiri** `Pengaturan → AI Trigger` (`/pengaturan/ai-trigger`): cara kerja, daftar trigger (aktif/mati
  langsung dari daftar, ubah, hapus), dan *Coba kalimat pelanggan* dengan pilihan dealer dan cabang, juga dengan AI.
  Terlihat oleh peran yang punya Chat atau Pengaturan; hanya CEO/admin yang mengubah. Kartu lama di Pengaturan dihapus.
- **Empat jenis** (`reply_rules.action`, migrasi 0041): `stock` (stok per cabang), `price` (harga sesuai tier dealer
  dari `products.prices`, else harga list), `credit` (limit, terpakai, sisa limit, tagihan terbuka, tagihan tertua —
  `metrics.CreditOf`), `order` (order terakhir 6 bulan dan fasenya). Tiap jenis punya placeholder sendiri dan dua
  draf (ya / tidak). Trigger bawaan baru: Tanya harga, Tanya limit & tagihan, Status order.
- Satu pesan bisa mendapat **beberapa kartu** (satu per jenis; trigger pertama per jenis menang). `MessageView.suggest`
  menjadi array.
- **Draf ditulis AI** (opsional per trigger, `ai_phrase` + `ai_instruction`): angka tetap dihitung Go
  (`internal/replyrule`); AI (`internal/replyai`, prompt `reply.md`) hanya merangkai kalimat dari `nilai` dan draf
  template. Draf AI ditolak bila menghilangkan angka draf template atau menambah angka yang tidak ada di fakta/pesan
  (`CheckDraft`); draf template tetap dipakai. Model dan kunci dari halaman API & model AI (Claude API atau ChatGPT API),
  else `LLM_PROVIDER`. PII dimasking, setiap panggilan tercatat di `llm_calls` (purpose `reply.ai`).
- Kapan AI menulis: job river `reply.ai` setelah pesan pelanggan baru masuk dan cocok dengan trigger ber-AI (bukan
  riwayat, bukan karyawan di grup; satu job per pesan); atau tombol **Tulis dengan AI** di kartu. Hasil disimpan di
  `reply_ai_drafts` dengan hash fakta — bila fakta berubah (stok tersinkron ulang), draf AI lama tidak tampil lagi.
  Kartu menampilkan "AI sedang menulis…" sampai 2 menit setelah pesan masuk. Retensi 30 hari.
- **Tidak ada pengiriman otomatis**: kartu hanya untuk sales; "Pakai jawaban" mengisi kolom ketik.

## Hubungan dengan ADR 0032
ADR 0032 membatasi *analisa* ke Claude lewat MCP. Draf AI Trigger bukan analisa: tidak menilai dealer atau membuat
usulan, hanya menyusun ulang kalimat dari fakta data, dan hanya bila CEO/admin mengaktifkannya per trigger. Bawaan
semua trigger: draf template (tanpa AI).

## Konsekuensi
- Kartu limit & tagihan dan status order hanya muncul di percakapan yang tertaut ke dealer.
- Harga mengikuti `products` (Odoo); bila belum tersinkron, kartu harga menyebut "Daftar harga belum tersinkron".
- Webhook Cloud API tidak memicu job `reply.ai` (nomor Baileys saja); tombol Tulis dengan AI tetap bisa dipakai.
