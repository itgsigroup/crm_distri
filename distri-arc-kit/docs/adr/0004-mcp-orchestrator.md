# ADR 0004 — MCP sebagai server DAN sebagai jalur orkestrasi
**Status**: diterima · 2026-10-05

## Konteks
Sam memakai Claude Desktop dan tim memakai ChatGPT. Mereka ingin bisa bertanya, menganalisis ulang, dan menjalankan Orchestrator dari sana — tanpa membuka celah pengiriman ke dealer.

## Keputusan
Distri ARC menyajikan MCP server (Streamable HTTP, SDK Go resmi) dengan tiga scope: `read`, `analyze`, `orchestrate`. Tool `orchestrator.*` memanggil `Orchestrator.Run` yang sama dengan jadwal internal; `orchestrator.input.get` + `orchestrator.submit` memungkinkan model eksternal bertindak sebagai mesin analisis (jalur `mcp`). Tidak ada tool keputusan; `allow_send=false` dipaku di kode. Mode klien MCP (Distri ARC memakai MCP server lain, mis. Odoo MCP) disiapkan sebagai `internal/mcp/client` untuk Stage 13+.

## Konsekuensi
- Satu jalur orkestrasi, dua pintu (internal, MCP) → tidak ada logika ganda.
- Audit penuh per panggilan; rate limit mencegah siklus bertubi-tubi.
