# ADR 0022 — Analisis terjadwal: Claude menganalisis lewat MCP sesuai jadwal cron

Status: diterima · 2026-10-07 · melengkapi ADR 0021

## Konteks
Setelah Claude bisa terhubung lewat MCP (ADR 0021), GSI ingin analisis berjalan sendiri pada waktu yang bisa diatur
(cron), tanpa seseorang membuka claude.ai. Produksi belum memakai kunci Anthropic (agen Orchestrator memakai template).

## Keputusan
- **Tabel `mcp_schedules`** (nama, prompt, cron 5 bagian WIB, izin tool, batas langkah, aktif) dan
  **`mcp_schedule_runs`** (laporan Markdown, langkah tool, token, biaya, status `running|ok|template|error`),
  unik per `(schedule_id, slot)` → tick yang berjalan ulang tidak menggandakan laporan.
- **Parser cron sendiri** (`internal/cron`, tanpa dependensi): `*`, angka, rentang, langkah, daftar, hari 0–7,
  makro `@daily` dll.; deskripsi Bahasa Indonesia ("Senin–Sabtu pukul 07.00"); jarak minimal 15 menit.
- **Worker**: `analyst.tick` tiap menit memajukan `next_run_at` dengan compare-and-set lalu mengantre `analyst.run`
  (MaxAttempts 1 — percobaan ulang memakan token). Slot yang terlewat > 3 jam (worker mati) dilewati.
- **Claude memakai tool MCP yang sama** (`internal/analyst`): server MCP dibuat di proses worker dan disambung lewat
  transport in-memory, dengan baris `mcp_clients` milik jadwal (kind `schedule`, tanpa token). Semua panggilan
  tercatat di `mcp_calls`/audit seperti klien lain; scope dicek; `actions_decide` tidak pernah ditawarkan.
- **Model**: Messages API (SDK Go resmi), adaptive thinking, loop tool-use sampai `max_steps` + 4 giliran;
  hasil tool dimasking (nomor/email di nilai string, angka rupiah tetap), placeholder dibuka kembali di laporan.
- **Kunci Claude API** diisi CEO di halaman (diperiksa ke Models API, disegel `SESSION_SECRET` di `secrets`),
  fallback `ANTHROPIC_API_KEY`. **Anggaran harian** (policy `mcp.analyst`, default Rp50.000) — lewat anggaran,
  tanpa kunci, atau model gagal → laporan **template** dari `data_ringkasan` + `penjualan_bulanan`.
- Laporan hanya dibaca manusia; tidak ada kiriman ke dealer.

## Akibat
- Biaya model hanya muncul setelah CEO mengisi kunci; dibatasi anggaran harian dan batas langkah per jadwal.
- Jadwal awal "Ringkasan pagi" (`0 7 * * 1-6`) dibuat oleh migrasi 0018.
