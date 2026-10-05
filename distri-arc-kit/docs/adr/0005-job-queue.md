# ADR 0005 — Antrean job & scheduler: river di PostgreSQL
**Status**: diterima · 2026-10-05

## Keputusan
`river` (riverqueue.com) untuk job (`cycle.run`, `wa.ingest`, `odoo.sync`, `metrics.recompute`, `outbox.send`, `retention.purge`) dengan periodic jobs untuk jadwal; `unique_opts` untuk idempotensi; satu `worker` process. Realtime ke UI lewat Postgres `LISTEN/NOTIFY` → SSE.

## Konsekuensi
- Tidak ada Redis; job terlihat di tabel `river_job` (bisa diinspeksi dari `arc ctl jobs`).
- Siklus Orchestrator memakai advisory lock agar tidak tumpang tindih.
