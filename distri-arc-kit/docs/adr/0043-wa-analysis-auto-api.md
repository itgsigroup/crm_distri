# ADR 0043 — Analisa WhatsApp otomatis lewat API: jam dan model dipilih, tanpa cadangan AI lain

Status: diterima · 2026-10-17 · Sam

## Konteks
Analisa WhatsApp berjalan tiap 2 jam lewat routine claude.ai (ADR 0037 rev.). Server bisa memakai Claude API atau ChatGPT API (ADR 0040), tetapi hanya sebagai opsi kedua bila routine terlambat. Sam ingin mengatur sendiri di Pengaturan → API & model AI **jam berapa** analisa otomatis jalan dan **model AI apa** yang dipakai, dan bila analisa otomatis dengan Claude gagal, ChatGPT **tidak** perlu mengambil alih.

## Keputusan
- Jadwal baru `mcp_schedules.key = 'wa_analysis_auto'` ("Analisa WhatsApp otomatis · API", runner server, migrasi 0046), prompt sama dengan routine. Default mati; jam default 08–16 tiap 2 jam, Senin–Sabtu.
- Jam dan on/off disimpan di baris jadwal (cron `0 <jam> * * 1-6` atau `* ` bila juga Minggu). AI (`claude` | `openai`) dan modelnya di policy `wa.auto`. Endpoint `PUT /api/ai/api/wa-auto` (CEO saja, diaudit); `GET /api/ai/api` mengembalikan `wa_auto` dan `wa_auto_runs`.
- Runner memakai AI + model pilihan itu untuk jadwal ini, terlepas dari opsi kedua. Run sukses hanya bila `orchestrator_submit` diterima (aturan yang sama dengan `wa_analysis`).
- **Tanpa cadangan**: run yang gagal tercatat Gagal dan menunggu jam berikutnya; tidak diulang dengan AI lain. Selama jadwal ini aktif, opsi kedua API untuk routine Analisa WhatsApp (`WAFallbackWorker`) juga diam. Opsi kedua untuk siklus Orchestrator tidak berubah.
- Routine claude.ai tidak disentuh (diatur di claude.ai). Bila hanya ingin analisa dari server, matikan routine di claude.ai.

## Konsekuensi
- Biaya token API dihitung dan dibatasi anggaran harian yang sama (Pengaturan → API & model AI).
- Bila routine claude.ai dan jadwal ini sama-sama aktif, analisa bisa jalan dua kali pada jam yang berdekatan.
