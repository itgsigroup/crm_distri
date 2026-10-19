# ADR 0046 — Analisis ulang oleh sistem GSI Orbit sendiri (tanpa Claude)

Status: diterima · 2026-10-19 · Sam · melonggarkan ADR 0025/0032 (wajib AI, hanya MCP) untuk siklus yang diminta dengan mesin ini

## Konteks
Sam: "pada orchestrator emang ini harus masuk Claude ya? kalau analisa dari sistem kita gabisa ya? tolong ubah logicnya
agar bisa." Dengan `llm.routing` = `mcp` + `require_ai` (ADR 0032), setiap Analisis ulang menunggu Claude dibuka di
claude.ai; tanpa itu siklus gagal.

## Keputusan
- Mesin ketiga **Sistem GSI Orbit**: `POST /cycles {via: "system"}` → siklus `via = 'system'` (migrasi 0049).
- Siklus ini berjalan penuh di server: agen memakai aturan Go mereka (template usulan dari angka glossary dan chat),
  ringkasan template, tanpa memanggil model apa pun (`routerFor` = nil; bacaan halaman dan ringkasan WhatsApp model
  dilewati) dan tanpa menerbitkan Input MCP. Untuk siklus itu saja, `require_ai` dan mode `mcp` diabaikan.
- UI: kotak Orchestrator punya pilihan **Sistem · MCP Claude · API & model AI**; Sistem jadi bawaan bila belum ada
  pilihan tersimpan dan API belum siap. Halaman Orchestrator: tombol utama **Analisis ulang (Sistem)**, tombol kedua
  **Lewat Claude · MCP**.
- Siklus per jam dan kebijakan `llm.routing` tidak berubah (tetap menunggu Claude/API).
- Usulan tetap butuh keputusan manusia; tidak ada yang terkirim sendiri.

## Konsekuensi
- Analisis ulang selalu bisa jalan seketika, tanpa biaya AI. Teks alasan dan draft pesan berasal dari template, bukan
  tulisan Claude — kualitas bahasa lebih sederhana; angka tetap dari rumus glossary.
- Riwayat analisis menandai siklus ini dengan jalur `SYSTEM`.
