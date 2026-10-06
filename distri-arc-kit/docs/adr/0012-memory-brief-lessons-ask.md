# ADR 0012 — Memori dealer, Ringkasan, pelajaran kalibrasi, Tanya
**Status**: diterima · 2026-10-06 · melengkapi `04-orchestrator.md` (Belajar) dan `07-api.md`

## Keputusan
1. **Memo = kalimat bersumber** (`dealers.memo_sentences`, maks 120 kata). Memo lama/impor diatribusikan per kalimat:
   topik siklus order → sinyal SO, topik kredit → invoice/pembayaran dan WA tentang bayar, sisanya → kecocokan kata
   dengan teks/anotasi sinyal; tanpa kecocokan → sinyal tempat memo itu ditulis (`memo_signal_ids`); tanpa keduanya
   kalimat dibuang. Bila ada sinyal baru, kalimat orbit dan kredit berangka diganti fakta terhitung; kalimat konteks
   tetap. LLM boleh merapikan bila `memory.Validate` lolos (setiap kalimat bersumber, sumber milik dealer, ≤ 120 kata).
2. **Ringkasan Orchestrator** ditulis siklus penuh (tabel `briefs`): 4 poin template (Go, tautan
   `[[dealer:slug|Nama]]`), `signal_ids` dari sinyal dealer yang disebut, dirapikan LLM hanya bila setiap tautan dan
   angka tetap ada. `/brief/today` membaca ringkasan hari ini; tanpa siklus → template.
3. **Pelajaran**: ≥ 3 penolakan 30 hari dengan agen, jenis, alasan (dan keluarga produk) yang sama → satu pelajaran
   (`calibration_lessons`) bercakupan tier yang sama / satu dealer / semua dealer, berlaku 14 hari; Sintesis menahan
   usulan yang cocok (bundle: dealer dalam cakupan dikeluarkan).
4. **Tanya (⌘K, `POST /ask`)**: router regex (risiko, lewat jadwal, kas, stok, jadwal; nama dealer → buka dealer;
   lainnya → ringkasan hari ini). Jawaban dihitung Go dengan tautan dealer dan sumber; LLM merangkai dengan validator
   yang sama dengan ringkasan.
5. **Seed menulis sinyal invoice terbuka** dengan kunci sinkron Odoo (`account.move:<id>:<write_date>`), sumber klaim kredit.
