# 05 · Agen — spesifikasi enam agen AI

## Kontrak
```go
type Input struct {
  Cycle    domain.CycleRef
  Policies domain.PolicySet
  Dealers  []domain.DealerView     // metrics_current + ringkasan 6 bulan + PIC + memo + 20 sinyal terbaru (masked)
  Stock    []domain.StockItem      // hanya agen stock
  Signals  []domain.Signal         // sinyal baru sejak siklus terakhir (scope)
  Examples []domain.FewShot        // dari calibration (edited proposals), maks 20
}
type Agent interface {
  Name() string                                  // "AI Order" …
  Kinds() []string                               // jenis proposal yang boleh dihasilkan
  Analyze(ctx context.Context, in Input, llm llm.Provider) ([]domain.Proposal, error)
}
```
Prinsip: **agen menghitung dulu dengan Go (kandidat, angka, aturan), baru memanggil LLM** untuk: memilih/menyusun kalimat `why`, `prep`, `preview` (draft WA dalam Bahasa Indonesia yang sopan, singkat, menyebut nama PIC dan angka nyata), dan `confidence`. LLM menerima JSON terstruktur dan wajib mengembalikan JSON sesuai schema (validasi; gagal → retry 1× lalu fallback template tanpa LLM dengan `confidence` 0.6).

Setiap proposal **wajib** `signal_ids` (≥ 1) dan `confidence`. PII (nomor WA, nama orang) dimasking (`<PIC_1>`, `<NO_1>`) sebelum ke provider eksternal dan di-unmask di hasil.

## Prompt sistem (bersama, `internal/llm/prompts/system.md`)
Berisi: identitas ("asisten distribusi GSI"), glossary ringkas (istilah + makna), aturan bahasa (Bahasa Indonesia, sapaan "Pak/Bu", tanpa emoji, maks 3 kalimat untuk draft WA, selalu sebut angka dan produk konkret), larangan (menjanjikan harga di bawah tier, menahan kiriman, mengancam, menyebut data dealer lain), format keluaran JSON.

## 1. AI Order (`internal/agents/order`)
- **Tujuan**: mengubah permintaan WA menjadi SO draft; menangkap nego harga.
- **Input**: sinyal `wa` inbound 24 jam dari thread dealer, harga tier dealer, stok cabang, sisa limit.
- **Logika Go**: ekstraksi produk/qty (LLM, schema `{items:[{product,qty}], intent:'order'|'ask_price'|'ask_stock'|'other'}`), cocokkan ke katalog (trigram), hitung total & margin, cek stok & limit.
- **Kinds**: `so_draft` (auto jika semua item cocok, stok ada, limit aman, harga = tier), `price_counter` (nego; **selalu approve**; counter tidak di bawah floor), `return` (RMA).
- **Contoh preview**: "Pak Budi, 10× Kamera IP 4MP + 1× NVR 16ch total Rp 18,4 jt harga tier A, stok Semarang siap, kirim Senin. Saya buatkan SO-nya ya?"

## 2. AI Follow-up (`agents/followup`)
- **Tujuan**: menjaga siklus order: jadwal order H-1, lewat jadwal, rekomendasi order.
- **Input**: `v_due_7d`, `v_drift`, komposisi order 6 bulan, stok kritis cabang, stok menua yang cocok.
- **Logika Go**: kandidat = due_in ∈ [0,7] atau lewat jadwal; susun rekomendasi order (glossary); tentukan akar terduga untuk lewat jadwal dari sinyal (template: "proyek belum cair" bila ada permintaan tempo; "harga vs marketplace" bila ada sebutan marketplace; default "porsi kecil").
- **Kinds**: `followup` (auto bila H-1, sisa limit aman, status Aktif/Key account, bukan follow-up ke-2; selain itu approve).
- **Tidak boleh**: menyebut diskon, menjanjikan tanggal kirim tanpa stok.

## 3. AI Kredit (`agents/credit`)
- **Tujuan**: menjaga sisa limit; menahan/menyetujui rilis; mengusulkan DP; mengusulkan kenaikan limit.
- **Input**: permintaan rilis (SO draft > sisa limit), invoice lewat tempo, on_time, pola bayar, riwayat limit.
- **Logika Go**: rilis di atas limit → opsi A (DP 50%), B (tahan sampai lunas), C (rilis sebagian); SOP-SEC-001 checklist (PO terverifikasi via telepon ke nomor terdaftar, alamat kirim konsisten) wajib `true` dari sinyal `manual` sebelum opsi apa pun; kenaikan limit bila on_time ≥ 90 dan room sering < 40% (≥ 3 dari 6 bulan).
- **Kinds**: `credit_release` (**selalu approve CEO**), `credit_limit` (approve), `credit_hold` (auto: menahan SO draft, bukan mengirim apa pun).
- **Tidak boleh**: melepas barang; mengubah limit di Odoo.

## 4. AI Stok (`agents/stock`)
- **Tujuan**: push stok menua ke dealer yang cocok; perluas product mix.
- **Input**: `stock_items` dengan `age_days > policy.aging_days`, kandidat dealer (glossary "Push stok"), stok kritis.
- **Logika Go**: kandidat & urutan dihitung Go; bundle = harga tier − diskon ≤ sehingga margin ≥ floor; "lebar baru" untuk dealer Key account dengan kategori kosong relevan.
- **Kinds**: `push_stock` (approve untuk harga bundle/promo; `auto` hanya menyusun daftar tanpa mengirim), `transfer` (usulan transfer antar cabang → approve gudang), `po_request` (stok kritis → approve purchasing).

## 5. AI Penagihan (`agents/collect`)
- **Tujuan**: pengingat invoice dengan nada mengikuti pola bayar; sinkron dengan jadwal order.
- **Input**: invoice `due_at - today ∈ [-30, 3]`, pola bayar, status dealer, jadwal order.
- **Logika Go**: H-3 ramah (auto), H+1..H+14 pengingat (approve bila nada tegas), > 14 hari skema cicilan (approve). Bila jadwal order ≤ 7 hari → sebut "order berikutnya bisa langsung diproses setelah pembayaran".
- **Kinds**: `collect` (auto hanya H-3 ramah), `installment` (approve).
- **Tidak boleh**: mengancam, menahan kiriman, menyebut denda yang tidak ada di termin.

## 6. AI Prospek (`agents/prospect`)
- **Tujuan**: nomor baru inbound → identifikasi → usul tier awal → follow-up pertama.
- **Input**: thread `kind='new'`, `identifications` (profil WA Business, Truecaller, Getcontact import manual CSV), kota, jenis usaha.
- **Logika Go**: skor identitas (kecocokan nama/org antar sumber), usul tier C cash, potensi dari dealer sejenis di kota sama (median omzet/bln).
- **Kinds**: `new_dealer` (approve: membuat dealer di Odoo), `price_list` (approve: kirim harga tier C).

## Matriks otonomi default (`policy.autonomy.matrix`)
| Agen | auto | approve | never |
|---|---|---|---|
| AI Order | `so_draft` (semua syarat) | `price_counter`, `return`, dealer baru | mengubah limit |
| AI Follow-up | `followup` H-1 limit aman | dealer At risk/Churn, follow-up ke-2 | menjanjikan harga |
| AI Kredit | `credit_hold` | `credit_release`, `credit_limit` | melepas barang tanpa approve |
| AI Stok | menyusun daftar | `push_stock`, `transfer`, `po_request` | di bawah floor margin |
| AI Penagihan | `collect` H-3 ramah | nada tegas, `installment` | mengancam / menahan kiriman |
| AI Prospek | identifikasi | `new_dealer`, `price_list` | mengirim harga sebelum approve |

## Uji wajib per agen (dengan `llm.Fake` deterministik)
- Menghasilkan proposal hanya untuk kandidat yang memenuhi aturan Go (tabel kasus dari glossary).
- Setiap proposal punya `signal_ids`, `confidence`, `kind ∈ Kinds()`.
- Draft WA ≤ 3 kalimat, menyebut nama PIC (unmasked) dan angka.
- Masking: nomor WA tidak pernah muncul di `llm_calls.input_hash` sumber (uji dengan provider spy).
