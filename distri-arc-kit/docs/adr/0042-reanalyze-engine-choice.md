# ADR 0042 — Analisis ulang: pilih mesin (MCP Claude atau API & model AI) dan halaman ikut diperbarui

Status: diterima · 2026-10-16 (Sam) · memperluas ADR 0040

## Konteks
Sam: "pada pop up orchestrator ketika analisa apakah sudah sesuai halaman tersebut? … ketika button analisa dipencet
maka halaman tersebut juga berubah sesuai analisa … analisa bisa lewat MCP atau model AI yang sudah dipilih pada
halaman model AI." Pemeriksaan: pemetaan halaman → scope sudah benar (Push stok → AI Stok + Analisa Stok, Kredit →
AI Kredit + AI Penagihan + Analisa Kas, Orbit/Segmen/Relasi → AI Follow-up + AI Kredit, Dealer → dealer itu,
Dashboard/lainnya → semua), tetapi (1) setelah siklus selesai Push stok, Kredit, KPI, Peta relasi dan Analisa WhatsApp
tidak di-refresh bila SSE terputus, (2) `cycle_done` dari kiriman Analisa Stok/Kas/WhatsApp lewat MCP mengakhiri
siklus di UI terlalu cepat, (3) tombol "API" memakai `LLM_PROVIDER` server, bukan pilihan di API & model AI, dan tidak
ada cara menjalankan siklus langsung dengan model pilihan itu.

## Keputusan
- Pop-up Orchestrator menampilkan **Layar ini: <halaman>** dan agen/analisa yang dijalankan, plus pilihan mesin:
  **MCP Claude** (claude.ai/Desktop; perintah disiapkan) atau **model dari API & model AI** (Claude API atau ChatGPT
  API, model yang dipilih di halaman itu). Pilihan disimpan per perangkat; model nonaktif bila kunci belum diisi atau
  anggaran harian habis (`GET /cycles/engines`, `analyst.APIReady`).
- `POST /cycles {via: "model"}`: siklus berjalan dengan `via = mcp` (Input agen dipublikasikan) dan job
  `cycle.mcp_analyst` langsung dijalankan — `analyst.Runner` memakai provider/model pilihan lewat tool MCP yang sama
  (jadwal "Analisis ulang lewat MCP (tombol)"), biaya tercatat di API & model AI. Tanpa kunci: 409 `no_api`.
  `via` kosong/`api`: router server bila ada dan kebijakan mengizinkan, else model pilihan bila siap, else MCP.
- Setelah siklus selesai (SSE atau polling), web me-refresh semua data yang bisa diubah siklus: keputusan, rencana,
  brief, dealer, orbit, segmen, relasi, KPI, agenda, stok (termasuk Analisa Stok), kredit (termasuk Analisa Kas),
  Analisa WhatsApp. `cycle_done` yang membawa `agent` (kiriman MCP per bacaan halaman) tidak lagi mengakhiri siklus.
