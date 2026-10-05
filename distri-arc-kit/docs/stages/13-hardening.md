# Stage 13 — Hardening: keamanan, retensi & partisi, backup/restore, observabilitas, deploy, runbook
**Baca**: `09-policies-security.md`, `10-testing-ops.md`, `02` (keandalan), ADR 0005.

## Deliverables
1. Keamanan: 2FA TOTP (ceo/admin), rate limit login, header Caddy (HSTS, CSP), `arc ctl check-env`, scan `govulncheck` + `npm audit` di CI.
2. Partisi bulanan `signals` & `chat_messages` (migrasi dengan pemindahan data), job `retention.purge` harian sesuai `policy.retention`, `arc ctl pdp export/delete`.
3. Backup `infra/backup.sh` (pg_dump + enkripsi, rotasi 30 hari) + `restore.sh`; uji restore otomatis mingguan di CI (dari dump seed).
4. Observabilitas: `/api/health` lengkap (db, queue depth, wa per nomor, odoo, llm), `/metrics` Prometheus, dashboard sederhana di Pengaturan → Status sistem (dari health), alert ke grup WA internal via outbox sistem (bukan ke dealer): WA terputus, siklus gagal 2×, antrean > 500.
5. Deploy: `infra/docker-compose.prod.yml`, `Caddyfile`, `infra/systemd/`, `docs/DEPLOY.md` (VPS Ubuntu 24.04, 2 vCPU 4 GB cukup), `docs/RUNBOOK.md` (insiden umum + langkah).
6. Performa: indeks dari EXPLAIN pada 5 query terberat (orbit, segmen, relasi, timeline, due); target p95 < 300 ms pada 50k sinyal (uji beban `k6` sederhana).

## Acceptance
- Restore dari backup menghasilkan metrik identik (uji).
- Purge menghapus chat > 90 hari tanpa menyentuh agregat (uji).
- Deploy prod di VPS uji: HTTPS, login 2FA, siklus per jam berjalan 24 jam tanpa error (log).
- `make check` + `make e2e` hijau.

Commit: `feat(stage-13): hardening, partisi & retensi, backup, observabilitas, deploy`
