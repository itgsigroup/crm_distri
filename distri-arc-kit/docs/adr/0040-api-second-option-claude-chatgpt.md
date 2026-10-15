# ADR 0040 — Claude API / ChatGPT API sebagai opsi kedua analisis

Tanggal: 2026-10-15 · Status: diterima · Melengkapi ADR 0032 (analisa hanya dari Claude lewat MCP) dan ADR 0037 (Analisa WhatsApp tiap 2 jam)

## Konteks
Sejak ADR 0032/0037 semua analisis ditulis Claude lewat MCP (claude.ai, Desktop, routine), dibayar langganan Claude. Kalau
Claude tidak menjawab dalam `MCP_WAIT`, agen dicatat Gagal dan halaman hanya berisi angka. Sam punya kunci Claude API dan
ChatGPT API dan ingin salah satunya menjadi **opsi kedua**, dengan total pembayaran token yang terlihat.

## Keputusan
- Halaman baru **Pengaturan → API & model AI** (`/pengaturan/api-ai`; menu untuk pemegang Pengaturan atau MCP Claude; yang
  mengubah hanya CEO). Isi: kunci Claude API dan ChatGPT API (diperiksa ke provider, disimpan terenkripsi di `secrets`,
  hanya ditampilkan sebagai petunjuk), model masing-masing, pilihan opsi kedua (Mati / Claude API / ChatGPT API),
  anggaran harian (kedua API bersama), total pembayaran token (hari ini / 7 hari / bulan ini / 30 hari, per provider,
  grafik harian, per model) dan riwayat saat API mengambil alih.
- Kebijakan `mcp.analyst` bertambah `fallback` (`off` | `claude` | `openai`, default `off`) dan `openai_model`
  (gpt-5, gpt-5-mini, gpt-4.1, gpt-4.1-mini; harga di `llm.Prices`, perkiraan).
- **Urutan**: Claude lewat MCP tetap utama. Saat tahap Analisis menunggu MCP, hook `Orchestrator.OnMCPWait` (worker)
  menjadwalkan job `cycle.mcp_analyst{fallback}` pada `MCP_WAIT` dan memperpanjang tunggu 10 menit. Job itu hanya
  berjalan untuk agen yang belum dijawab; API memakai runner Analisis terjadwal (tool MCP yang sama, scope, masking,
  validasi `orchestrator_submit`) lewat jadwal tersembunyi "API cadangan · siklus Orchestrator".
- Analisa WhatsApp: job tiap 30 menit menjalankan prompt jadwal `wa_analysis` lewat API ("API cadangan · Analisa
  WhatsApp") bila tidak ada run berhasil > 2½ jam dan API belum mencoba dalam 2 jam terakhir.
- ChatGPT memakai Chat Completions + function calling lewat `net/http` (tanpa SDK baru). Setiap panggilan dicatat di
  `llm_calls` dengan provider `openai`, token dan biaya.
- Tombol Analisis ulang tidak lagi langsung menjalankan Claude di server saat kunci tersimpan; server hanya bekerja
  sebagai opsi kedua.

## Konsekuensi
- Dengan opsi kedua aktif, siklus yang tidak dijawab Claude lewat MCP tetap mendapat analisis AI (berbayar API, dibatasi
  anggaran harian). Dengan opsi kedua mati, perilaku sama seperti ADR 0032.
- Siklus via MCP bisa berlangsung sampai `MCP_WAIT` + 10 menit.
- Harga model adalah perkiraan; kurs `LLM_IDR_PER_USD`.
