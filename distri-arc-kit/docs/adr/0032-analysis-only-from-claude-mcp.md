# ADR 0032 — Analisa hanya dari Claude lewat MCP

Status: diterima · 2026-10-13 (Sam) · mempersempit ADR 0025 (wajib AI) dan ADR 0026 (Analisa WhatsApp)

## Konteks
Permintaan: di Orchestrator, Pusat kendali, Chat, Push stok, dan Kredit · kas, hapus semua analisa yang bukan dari
Claude lewat MCP. Inventaris menemukan analisa dari aturan Go dan template (deteksi "permintaan" dengan kata kunci,
langkah "Balas chat … minta order" di Rencana, kalimat feed "6 agen AI", "pesan penutup — tidak perlu dibalas",
"minta tempo", proposal template saat Claude tidak menjawab), dari model server (ringkasan WhatsApp lewat API), dan
dari data contoh (anotasi bubble, "Rekomendasi balasan (AI)", tag chat) — sebagian berlabel "AI".

## Keputusan
- Kebijakan `llm.routing` = `mode: "mcp"`, `require_ai: true` (migrasi 0030; default baru `mcp`). `LLMRouting.MCPOnly()`.
- Dalam mode MCP: Analisis selalu lewat `analyzeViaMCP` tanpa fallback template (agen tanpa jawaban Claude = tanpa
  usulan); `MakeCollect` tidak menulis tagihan sendiri; Analisa WhatsApp (ringkasan, baris per percakapan, catatan per
  bubble) dan Analisa Stok/Kas tidak ditulis model server — hanya `orchestrator_submit` Claude.
- UI kelima halaman menampilkan **hanya**: (1) yang ditulis Claude lewat MCP — usulan di Keputusan, ringkasan
  WhatsApp, ringkasan per percakapan, catatan per bubble, Analisa Stok/Kas; dan (2) angka/fakta yang diukur dari data
  (jumlah chat, lama menunggu, waktu balas, KPI, piutang, stok, prediksi kas dan dealer cocok menurut rumus glossary).
  Dihapus: bar/matriks/filter "permintaan", kartu Minta order / Tanya stok & harga / Rilis kredit & retur, feed
  "6 agen AI", Rencana hari ini dari chat, baris AI di grid sales, anotasi & rekomendasi balasan contoh, tag chat,
  "pesan penutup", "minta tempo", saran template di panel lead. Tombol Analisis ulang di halaman-halaman ini lewat MCP.
- Data contoh (seed) tetap `mode: "both"`, `require_ai: false` agar demo lokal dan test berjalan tanpa Claude.

## Konsekuensi
- Tanpa Claude yang menjawab lewat MCP (klien Claude, atau Claude di server dengan kunci Analisis terjadwal), siklus
  gagal dan halaman hanya berisi angka. Siklus terjadwal tiap jam ikut menunggu Claude.
- Pengaturan → Jalur analisis masih bisa diubah oleh admin; mengubahnya keluar dari keputusan ini.
