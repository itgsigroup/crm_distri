# ADR 0051 — AI Trigger: semua rekomendasi ditulis AI saat trigger cocok; ChatGPT boleh di sini

Status: diterima · 2026-10-19 · Sam · pengecualian sempit atas ADR 0048 (tanpa ChatGPT)

## Konteks
Kartu stok/harga/limit/status order di Chat (AI Trigger, ADR 0041) dikenali dari kata kunci dan angkanya dihitung
sistem; drafnya template, kecuali trigger yang mengaktifkan "Jawaban ditulis AI". Sam meminta rekomendasinya dari AI
dan real-time, "pakai ChatGPT juga boleh, tiap kali ada trigger kata stok, harga dll — tidak perlu analisa yang lain
jadi lebih murah". Routine Claude (ADR 0049) terlalu lambat (30–60 detik) dan terbatas (30 run/jam) untuk per pesan.

## Keputusan
- Semua trigger memakai AI (`reply_rules.ai_phrase = true`, juga bawaan trigger baru; migrasi 0051). Pengenalan tetap
  kata kunci trigger: AI dipanggil **hanya** untuk pesan pelanggan yang cocok, lewat job `reply.ai` saat pesan masuk.
- Mesin penulis rekomendasi dipilih di halaman AI Trigger (policy `reply.ai`): **ChatGPT** (bawaan `gpt-4.1-mini`,
  juga nano / 4o-mini / 5-mini) atau **Claude** (Haiku 5.5 / Sonnet 5.5). Terpisah dari mesin analisa.
- ChatGPT dikecualikan dari ADR 0048 **hanya** di sini: `analyst.LoadOpenAIKeyAITrigger` membaca kunci untuk AI
  Trigger; `LoadOpenAIKey` (analisa, opsi kedua, Analisa WhatsApp otomatis) tetap kosong. Bila Claude dipilih tanpa
  kunci Claude, tidak ada peralihan diam-diam ke ChatGPT — draf template dipakai.
- Tetap: angka dari sistem, draf AI yang menghilangkan atau menambah angka ditolak (`CheckDraft`), PII dimasking,
  panggilan tercatat di `llm_calls` (purpose `reply.ai`), tidak ada yang terkirim otomatis.

## Konsekuensi
- Biaya mengikuti jumlah pesan yang cocok trigger saja (gpt-4.1-mini ±Rp 15 per draf). Anggaran harian di API & model AI.
- Pesan yang tidak memakai kata kunci trigger tetap tidak mendapat kartu; menambah kata kunci di halaman AI Trigger.
