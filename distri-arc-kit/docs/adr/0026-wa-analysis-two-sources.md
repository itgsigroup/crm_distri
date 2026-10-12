# ADR 0026 — Dua analisa: BigQuery + WhatsApp

Status: diterima · 2026-10-12

## Konteks
Pusat kendali dan Orchestrator hanya menampilkan analisa dari data BigQuery (SO, pembayaran, stok). Chat WhatsApp
sudah masuk sebagai sinyal `wa`, tetapi hanya AI Order yang memakainya; tidak terlihat siapa yang menunggu balasan,
apa yang diminta dealer, dan seberapa cepat sales membalas. Permintaan: "analisa dari WhatsApp dan dari BigQuery,
jadi 2 analisa", dan hasil WhatsApp masuk Rencana.

## Keputusan
- Setiap **siklus penuh** (scope `all`) membaca dua sumber. Tahap Ingest memanggil `readWA`: pesan 7 hari terakhir
  dari percakapan `dealer` dan `new` (grup tidak dibaca; DM antar karyawan memang tidak disimpan).
- `internal/waanalysis` (Go murni): per percakapan — niat (ekstraktor yang sama dengan AI Order: order, tanya harga,
  tanya stok, rilis kredit, retur), barang yang diminta, pesan masuk/keluar, menunggu sejak, median waktu balas,
  prioritas; `Summarize` menghitung total dan per sales. Semua angka dari Go.
- Disimpan per siklus: `wa_analyses` (satu baris) dan `wa_insights` (per percakapan, PK `cycle_id, thread_id`,
  provenance `signal_ids`, `proposal_ids`). Disimpan di Ingest (terlihat walau Analisis gagal) dan diperbarui di
  Keputusan setelah usulan siklus ini tertaut lewat `signal_ids`. Siklus ulang mengganti, tidak menggandakan.
- **Rencana hari ini**: percakapan yang menunggu > 2 jam (`ReplyDue`) dan belum dijawab usulan agen yang terbuka
  menjadi langkah "Balas chat" (agen `Analisa WhatsApp`, `approve`, `link = chat:<thread>`), maks 3 langkah, sebelum
  langkah approve lain. Tidak ada pengiriman: tombolnya membuka Chat. `/plan/today` menandainya selesai begitu
  balasan keluar, dan menempatkannya di halaman sales pemilik dealer / nomor.
- **AI** (tahap Belajar): bila server punya model, satu panggilan `wa.summary` (≤ 20 percakapan, nama orang dan sales
  dimasking, dicatat di `llm_calls`) menulis ringkasan dan satu baris per percakapan. Hanya jawaban model yang
  disimpan (ADR 0025); tanpa model, tab menampilkan angka saja.
- API `GET /api/wa/analysis` (dijaga halaman Chat karena mengutip pesan pelanggan; dibatasi per sales seperti
  Pusat kendali; status menunggu dibaca ulang saat permintaan). Tool MCP baca `wa.analisis` (PII dimasking).
- UI: tab **Analisa BigQuery | Analisa WhatsApp** (`?analisa=wa`) di Pusat kendali dan Orchestrator.

## Konsekuensi
- Rencana pada data contoh bertambah satu langkah (Graha: PO kemarin 16.20 belum dibalas): 9 langkah, bukan 8.
- Ambang 2 jam, jendela 7 hari, dan maks 3 langkah masih konstanta di `waanalysis`; bisa dijadikan kebijakan.
- Jalur MCP (tanpa model di server) belum menulis ringkasan WhatsApp; Claude dapat membacanya lewat `wa.analisis`.

## Pembaruan 2026-10-12 — tanpa API key, lewat MCP
- Langkah "Balas chat" ditulis sudah di **Ingest** (`writeChatSteps`), jadi tetap masuk Rencana walau siklus gagal di
  Analisis (tidak ada jawaban AI). Keputusan menulis ulang seluruh Rencana bila siklus sampai sana.
- Ringkasan AI WhatsApp ditulis **Claude lewat MCP**: `orchestrator_input_get` dengan agent `Analisa WhatsApp`
  (chat dimasking, mapping disimpan di `cycle_inputs`) dan `orchestrator_submit` dengan `overview` + `threads`.
  Divalidasi terhadap Input, di-unmask, disimpan `source = ai`. Bisa dikirim kapan saja setelah Ingest, juga untuk
  siklus yang gagal. Prompt "Analisis ulang lewat MCP" menyertakan langkah ini.
