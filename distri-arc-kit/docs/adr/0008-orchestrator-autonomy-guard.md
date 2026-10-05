# ADR 0008 — Orchestrator: satu siklus aktif, otonomi yang menyentuh dealer, rencana harian
**Status**: diterima · 2026-10-05 · melengkapi ADR 0004 dan `04-orchestrator.md`

## Konteks
Matriks otonomi (`policy.autonomy.matrix`) mengizinkan beberapa langkah `auto` yang mengirim pesan ke dealer
(follow-up H-1, pengingat H-3). CLAUDE.md §2 mewajibkan keputusan manusia yang tercatat sebelum apa pun sampai ke dealer,
dan §3 meminta pertanyaan sebelum mengirim ke dealer sungguhan. Selain itu, siklus berjalan lintas banyak transaksi,
sehingga `pg_advisory_xact_lock` saja tidak cukup untuk menjamin satu siklus aktif dan memberi `409` yang konsisten.

## Keputusan
1. **Satu siklus aktif** dijamin oleh indeks unik parsial `cycles_one_active` (status `queued`/`running`). `POST /cycles`
   menulis siklus `queued` + job `cycle.run` dalam satu transaksi; pelanggaran indeks → `409 cycle_running`. Worker
   memegang `pg_try_advisory_lock(42, hashtext(current_schema()))` selama siklus (kunci per skema agar skema test dan
   tenant terpisah tidak saling menahan). Saat worker start, siklus `running` yang tertinggal ditandai `failed`.
2. **Otonomi = matriks + guard** (`autonomy.guard`, default `{min_confidence: 0.8, dealer_messages: "confirm"}`):
   `auto` hanya bila matriks mengizinkan, confidence ≥ batas, dealer bukan At risk/Churn, sisa limit aman untuk jenis
   yang menambah exposure (follow-up, push), tidak tersentuh aturan kontensi (`credit_over_stock`, `margin_floor`,
   `one_owner`), dan syarat per jenis (follow-up H-1 dan bukan ke-2, pengingat bernada ramah, SO draft lengkap).
   `credit_release`/`credit_limit` tidak pernah otonom (PUT `/policies/autonomy` menolaknya).
3. **Langkah otonom yang menyentuh dealer menunggu satu klik**. Dengan `dealer_messages: "confirm"` langkah itu
   tersusun di Rencana hari ini berlabel *otonom* (terjadwal), draft siap, tetapi baru dikirim saat manusia menekan
   *Jalankan sekarang* — keputusan tercatat per proposal (`decided_by`), lewat outbox seperti approve biasa. Langkah
   otonom yang tidak menyentuh dealer (SO draft, credit hold) langsung disetujui sistem. Nilai `"auto"` (kirim pada
   jam slotnya, `decided_by` kosong + audit `proposal.auto`) hanya boleh dinyalakan pemilik setelah pilot (Stage 11/14).
4. **Rencana hari ini** (`plan.Build`, fungsi murni): langkah otonom dulu (dikelompokkan: rekomendasi order H-1,
   pengingat H-3, follow-up yang menunggu pembayaran), lalu yang butuh approve: rilis kredit yang menahan kiriman,
   pengingat bernada tegas, satu bundle stok bernilai terbesar, follow-up dealer lewat jadwal, nomor baru. Keputusan
   yang menjawab permintaan (harga, retur, limit) tetap di *Keputusan*; follow-up 2–7 hari tetap di *Jadwal order*.
   Slot mulai setengah jam berikutnya (≥ 07.30), approve ≥ 08.30, tiap 30 menit, maksimal 10 langkah.
5. **Agen v1 Stok, Penagihan, Prospek ditarik ke Stage 06**: acceptance 06 membutuhkan `collect` (Nusa), bundle LED dan
   nomor baru di rencana. Stage 09 memperdalamnya (transfer/PO, identifikasi Truecaller/CSV, layar Push stok & Kredit).
6. **Siklus ber-scope** (layar, agen) melihat proposal hari ini dari agen di luar scope sebagai *jangkar* aturan
   Sintesis, sehingga "satu tawaran per dealer" dan "tagih dulu" tetap berlaku antar siklus; jangkar tidak disimpan ulang.

## Konsekuensi
- Rencana pada seed persis 8 langkah (3 otonom, 5 approve) dan Keputusan "7 otonom · 4 ke Anda" seperti mockup.
- Tidak ada pesan otomatis ke dealer sampai pemilik mengubah `autonomy.guard.dealer_messages` secara sadar.
- Teks mockup "7 langkah dijalankan sendiri" ditulis "7 langkah otonom" agar jujur terhadap guard ini.
