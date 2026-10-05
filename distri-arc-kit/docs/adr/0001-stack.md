# ADR 0001 — Stack: Go · React · PostgreSQL
**Status**: diterima · 2026-10-05

## Konteks
Distri ARC Orbit dijalankan single-tenant di VPS/mini PC GSI, ≤ 50 pengguna, dengan worker yang berjalan terus (ingest WA, sync Odoo, siklus Orchestrator tiap jam). Tim pemelihara kecil; Sam ingin sistem yang "berjalan sendiri" dan mudah dideploy.

## Keputusan
- **Backend Go 1.23**: satu binary (`api`, `worker`, `ctl`), `chi`, `pgx/v5`, `sqlc`, `goose`, `river` (job queue di Postgres), `slog`. Alasan: binary tunggal, konkurensi untuk agen paralel, memori kecil, `whatsmeow` (WhatsApp) dan SDK MCP resmi tersedia di Go → tidak perlu sidecar Node.
- **PostgreSQL 16**: data + antrean job (river) + NOTIFY untuk realtime → satu dependensi infrastruktur. JSONB untuk payload dan kebijakan; partisi untuk tabel besar.
- **Frontend React 19 + TS + Vite**: SPA dilayani Caddy; CSS Modules + token dari mockup (tanpa Tailwind supaya visual mockup dipindah apa adanya); TanStack Query + SSE.

## Konsekuensi
- Tidak ada Redis/RabbitMQ. Skala hingga ~ratusan ribu sinyal/bulan cukup dengan partisi.
- Tim perlu Go; kode domain dibuat sangat eksplisit (tanpa reflection/ORM) agar mudah dibaca.
- Alternatif ditolak: Python/FastAPI (kit v3 ARC) — boleh dipakai untuk prototipe, tapi worker jangka panjang dan WA lebih stabil di Go; NestJS — lebih banyak dependensi.
