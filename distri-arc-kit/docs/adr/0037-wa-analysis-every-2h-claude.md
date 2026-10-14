# ADR 0037 — Analisa WhatsApp Pusat kendali oleh Claude lewat MCP setiap 2 jam

Status: diterima · 2026-10-14 (Sam)

## Keputusan
- Scope siklus baru `screen:wa`: membaca semua percakapan dan menulis Analisa WhatsApp (total, per percakapan, bacaan
  untuk Claude) **tanpa agen** — tidak ada yang menunggu enam jawaban setiap 2 jam.
- Jadwal analisis terjadwal (ADR 0022) ber-kunci `wa_analysis` dari migrasi 0035: "Analisa WhatsApp · tiap 2 jam",
  cron `0 */2 * * *` WIB, scope read + orchestrate. Claude: `orchestrator_reanalyze screen:wa` → `orchestrator_input_get`
  "Analisa WhatsApp" → `orchestrator_submit` (overview, threads, messages). Tidak pernah mengirim apa pun ke pelanggan.
- Run jadwal ini **berhasil hanya bila** `orchestrator_submit` Claude diterima; mode template (tanpa kunci / anggaran
  habis), Claude selesai tanpa submit, atau submit ditolak = gagal dengan alasan.
- Pusat kendali menampilkan label: "Analisa Claude berhasil · waktu" / "gagal · alasan" / "sedang menganalisa", plus
  jadwal berikutnya dan tautan Riwayat. MCP Claude → Analisis terjadwal menampilkan riwayat run (waktu, hasil,
  keterangan, biaya) dan laporan per run. Jadwal bisa dijeda, tidak bisa dihapus.

## Konsekuensi
- Butuh kunci Claude API di MCP Claude → Analisis terjadwal → Atur mesin, dan anggaran harian yang cukup untuk 12 run
  per hari (kebijakan `mcp.analyst.daily_budget_idr`).
