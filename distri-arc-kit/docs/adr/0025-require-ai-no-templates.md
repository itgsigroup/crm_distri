# ADR 0025 — Wajib AI: tanpa template, tanpa AI = Gagal

Status: diterima · 2026-10-10 · menggantikan perilaku fallback template di ADR 0008/0022 untuk produksi

## Konteks
Tanpa API key (`LLM_PROVIDER=fake`) agen menulis alasan, draft, ringkasan, dan laporan dari kalimat template.
Sam: "analisa AI-nya jangan sampai template — analisa semuanya dari AI; kalau tidak dari AI MCP berarti gagal."

## Keputusan
- Kebijakan `llm.routing.require_ai` (bawaan **aktif**; data contoh `seed.Run` mematikannya agar demo/test tetap jalan).
- Siklus Orchestrator:
  - Ada model di server (Claude API): usulan yang teksnya tidak ditulis model (`payload.text_by != ai`) dibuang; agen tanpa
    usulan AI = **failed**.
  - Tidak ada model: Analisis memakai **jalur MCP** — Input tiap agen diterbitkan, siklus menunggu `MCP_WAIT`
    (bawaan 10 menit) untuk `orchestrator_submit` dari Claude. Agen yang tidak dijawab = **failed** (tanpa template);
    tidak ada satu pun = siklus **failed** dengan catatan cara memintanya ke Claude.
  - Usulan gabungan (MakeCollect) di Sintesis hanya bila ditulis AI.
- Ringkasan Orchestrator hanya disimpan bila ditulis AI; `/brief/today` tidak menampilkan kalimat template (hanya angka)
  dan mengabaikan ringkasan template yang tersimpan sebelumnya.
- Analisis terjadwal tanpa kunci / lewat anggaran / model gagal = run **error**, tanpa laporan template.
- Angka (metrik Orbit, jadwal order, limit) tetap dihitung di Go — bukan template, bukan LLM (CLAUDE.md §2).

## Konsekuensi
Tanpa API key, siklus terjadwal tiap jam hanya berhasil bila ada Claude (lewat MCP) yang mengirim analisis dalam batas
tunggu; selain itu tercatat Gagal di Riwayat analisis. Usulan lama berbasis template yang sudah ada tidak dihapus.
