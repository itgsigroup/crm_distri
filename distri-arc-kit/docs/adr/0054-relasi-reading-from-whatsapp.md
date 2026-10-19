# ADR 0054 — Analisa Relasi dari isi chat WhatsApp; bacaan halaman mengikuti halaman yang dianalisa

Status: diterima · 2026-10-19 · Sam · pola yang sama dengan ADR 0052 (Orbit) dan 0053 (Segmen)

## Konteks
Sam: "peta relasi 3D ubah analisa dari WhatsApp, pastikan sesuai dengan MD". MD (00-overview): Peta relasi =
"seberapa dekat sales dengan dealer (interaksi WA + order)"; komponen RelasiMap3D, Filters, Pairs, Insights. Garis
peta sudah dihitung dari WA + order; kartu "Pola relasi" berlabel "dibaca GSI Orbit" berisi aturan, bukan analisa.

## Keputusan
- Bacaan halaman **Analisa Relasi** (agent `Analisa Relasi`, screen `relasi`, migrasi 0054). Input
  (`relasiReading`): ≤ 20 dealer — yang dichat dua sales, yang chatnya menipis (15 hari terakhir < separuh 15 hari
  sebelumnya), order besar (≥ Rp 60 jt) tapi chat < 3, lalu yang paling banyak chat — dengan sales pemilik, pesan per
  sales, PIC aktif, dan 8 pesan terakhir (pengirim sales disebut namanya, dimasking). Prompt `relasi_read.md`.
- Ditulis Claude lewat MCP (routine, ADR 0049). Halaman: kartu **Analisa Relasi** (ringkasan + catatan per dealer,
  klik → dealer). Kartu **Pola relasi** tetap (sinyal terukur) dengan label jujur "dihitung dari interaksi WA + order".
- **Bacaan per halaman**: `ScreenReadingsIn(scope, agents)` — Analisis ulang di Orbit/Segmen/Peta relasi meminta
  bacaan halaman itu saja (cepat); siklus penuh meminta ketiganya. `MCPPrompt` menerima scope (API, hook routine,
  cadangan); kotak Orchestrator menampilkan bacaan yang sama.
- Sesuai MD dan tidak diubah: node sales biru, dealer berwarna skor, ukuran = interaksi/bulan, periode 30–180, klik
  dua kali → dealer, cadangan 2D; Lihat dari (filter), Pasangan terkuat, Pola relasi.
