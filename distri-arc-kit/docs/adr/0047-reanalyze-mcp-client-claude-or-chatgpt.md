# ADR 0047 — Analisis ulang lewat MCP: Claude atau ChatGPT, tanpa kunci API

Status: diterima · 2026-10-19 · Sam · bagian ChatGPT dicabut oleh ADR 0048 · menggantikan pilihan mesin "API & model AI" di UI (ADR 0042) dan opsi kedua API
(ADR 0040) untuk siklus dari tombol Analisis ulang

## Konteks
Sam: "jangan pakai api key claude, kita pakai MCP Claude seperti Orchestrator di Pusat kendali, tambahkan pilih model
AI Claude atau ChatGPT; jika pilih Claude maka tidak pilih ChatGPT; intinya jangan pakai ChatGPT."
Produksi punya `mcp.analyst.fallback = openai`: siklus yang tidak dijawab Claude lewat MCP diambil alih ChatGPT API.

## Keputusan
- Mesin Analisis ulang di kotak Orchestrator: **Sistem** (ADR 0046) atau **MCP**. Mesin "API & model AI" dihapus dari UI
  (pilihan lama tersimpan dibaca sebagai MCP). Halaman Orchestrator: hanya MCP.
- Di bawah MCP ada pilihan **AI: Claude | ChatGPT** (bawaan Claude), model per AI (Claude: Opus/Sonnet/Haiku/Fable 5.x;
  ChatGPT: GPT-5, GPT-5 mini, GPT-4.1) dan effort — disimpan per perangkat, dipakai bersama oleh kotak dan halaman.
- `POST /cycles {via: "mcp", ai: "claude" | "chatgpt", model, effort}`: prompt yang sama dibuka di claude.ai
  (`engine: claude_ai`) atau chatgpt.com (`engine: chatgpt`); keduanya klien MCP, tanpa kunci API.
- **Satu AI per siklus**: siklus dari tombol Analisis ulang (trigger manual + model dipilih) tidak pernah diambil alih
  opsi kedua API (`worker.Fallback.Hook` melewatinya; `mcp.fallback` selalu kosong). Pilih Claude = ChatGPT tidak dipakai.
- Pop-up analisis kini membaca state siklus yang hidup (sebelumnya membaca state kosong karena dirender di atas
  `OrchProvider`, sehingga tampil "Selesai"/"API" saat siklus masih berjalan).

## Konsekuensi
- Analisa lewat MCP butuh klien yang menjalankan prompt: tombol **Buka Claude / Buka ChatGPT** di pop-up membuka prompt
  itu; tanpa itu agen menunggu sampai `MCP_WAIT` lalu gagal (tidak ada cadangan API).
- Siklus per jam dan Analisa WhatsApp otomatis tidak berubah: opsi kedua API mereka tetap mengikuti Pengaturan → API &
  model AI (produksi: ChatGPT). Mematikannya = setel "Opsi kedua: Mati" di halaman itu.
