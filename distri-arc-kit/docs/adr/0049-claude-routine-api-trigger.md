# ADR 0049 — Siklus menunggu Claude memicu routine claude.ai (tanpa membuka Desktop, tanpa kunci API)

Status: diterima · 2026-10-19 · Sam ("kerjakan pilihan 1")

## Konteks
Analisis ulang lewat MCP Claude hanya jalan bila seseorang membuka Claude (claude.ai/Desktop) dan menekan Enter. Tanpa
itu siklus gagal setelah `MCP_WAIT` — siklus per jam gagal berturut-turut (status server "degraded"). Kunci API Claude
dan ChatGPT tidak boleh dipakai (ADR 0047/0048). Routine Claude Code di claude.ai berjalan di cloud Claude dengan
langganan, memakai konektor CRM-DISTRI, dan bisa dipicu lewat **API trigger** (`POST
https://api.anthropic.com/v1/claude_code/routines/{id}/fire`, `Authorization: Bearer sk-ant-oat01-…`,
`anthropic-version: 2023-06-01`, body `{"text": …}` → `claude_code_session_url`).

## Keputusan
- Routine **"GSI Orbit · Analisis Orchestrator (dipicu server)"** (`trig_01VvHFKdz9UQj3aHMyoQ9isk`, Claude Sonnet 5.5,
  konektor CRM-DISTRI). Prompt tetap: jalankan perintah siklus di blok `<routine-fire-payload>` hanya dengan
  `orchestrator_input_get`, `orchestrator_submit`, `orchestrator_status`; tidak pernah mengirim ke pelanggan atau
  memutuskan. Jadwal cron wajib di API routine → placeholder setahun sekali (1 Jan); tanpa payload routine hanya
  membaca status lalu selesai.
- Token API trigger hanya bisa dibuat di UI claude.ai: CEO menyimpan ID routine + token di **MCP Claude → Routine
  Claude** (`PUT /api/ai/routine`, tersegel di `secrets` `claude.routine_id`/`claude.routine_token`; `GET` tidak pernah
  mengembalikan token; `POST /api/ai/routine/test` memicu satu run uji yang hanya membaca status).
- Saat tahap Analisis mulai menunggu MCP, hook worker (`Fallback.Hook`) **lebih dulu** memicu routine dengan
  `MCPPrompt` siklus itu (agen yang ditunggu) dan memperpanjang tunggu `RoutineWindow` = 20 menit. Berlaku untuk tombol
  Analisis ulang dan siklus per jam. Bila routine terpicu, opsi kedua API tidak dijadwalkan. Bila gagal (token ditolak,
  429), siklus kembali menunggu Claude manual seperti sebelumnya. Timeout job siklus = tunggu MCP + 20 + 5 menit.
- Event SSE `routine_fired {cycle_id, ok, url|error}`: pop-up analisis menulis "Claude sedang bekerja di claude.ai
  (routine)" dengan tautan **Lihat Claude bekerja**; tombol Buka Claude hanya muncul bila routine belum diisi atau gagal.

## Konsekuensi
- Analisis berjalan sendiri tanpa membuka apa pun; pemakaian dihitung ke kuota langganan Claude Code. Batas: API fire
  30/jam per routine dan 100/jam per akun — siklus per jam (15/hari) + Analisis ulang jauh di bawahnya.
- Routine memakai model routine (Sonnet 5.5), bukan model di dropdown; dropdown tetap untuk analisa manual di claude.ai.
- API fire tidak punya kunci idempoten: hook memicu sekali per siklus. Fitur routine masih research preview.
