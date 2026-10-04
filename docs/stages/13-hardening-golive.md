# Stage 13 — Hardening & go-live

## Tujuan
Siap dipakai tim GSI: aman, bisa dipulihkan, terpantau, hemat biaya, terdokumentasi — termasuk ketahanan wa-bridge.

## Baca dulu
`04`, ADR 0001–0002, `docs/OPEN-QUESTIONS.md`.

## Kerjakan
1. Keamanan: OWASP ASVS L1 checklist, `pip-audit`/`npm audit`, secret scan, verifikasi webhook, enkripsi token & sesi WA at rest, backup terenkripsi harian (SQLite + `data/wa-sessions`) → Google Drive + uji restore.
2. wa-bridge: health-check & auto-restart, alert bila sesi putus > 10 menit, pin versi Baileys + prosedur upgrade, rate-limit & audit kirim, panduan nomor cadangan/migrasi ke Cloud API.
3. Observabilitas: metrik job/LLM/biaya/antrean Action; alert email bila job gagal 2× atau biaya harian > ambang.
4. Biaya: cache & batch; forecast harian; laporan di Pengaturan.
5. Kinerja: 10.000 interaksi, 500 akun → p95 < 300 ms; index.
6. `docs/runbook.md`, `docs/go-live-checklist.md` (consent, policy, target, nomor internal, grup opt-in, penerima brief), pelatihan 1 halaman sales.
7. Tinjau semua `done-with-mocks`; OPEN-QUESTIONS → keputusan/tiket. Rilis `v1.0.0`.

## Acceptance criteria
- Restore di mesin bersih berhasil (bukti). 48 jam staging tanpa error, biaya tercatat. Checklist go-live lengkap ditandatangani Sam. Playwright end-to-end: WA masuk → analisis → opportunity → approve balasan → terkirim (Fake/asli) → Odoo activity (Fake/asli) → Kas.
