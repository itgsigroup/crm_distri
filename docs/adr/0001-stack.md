# ADR 0001 — Stack teknologi ARC
Status: diterima (tahap 00), **sebagian digantikan ADR 0003** (stack Go + React + PostgreSQL). Boleh diubah hanya lewat ADR baru + persetujuan Sam.

## Konteks
GSI ~30 staf, 4 cabang, single-tenant. Preferensi pemilik: self-hosted, dependensi minimal, Mini PC/Ubuntu, SQLite, biaya rendah, mudah dirawat oleh tim kecil. Beban: ±2.000 panggilan LLM ringan/hari, ±60 berat/hari, < 50 pengguna bersamaan.

## Keputusan
- **Backend**: Python 3.12, FastAPI, SQLAlchemy 2 + Alembic, SQLite (WAL mode) — cukup untuk skala ini; jalur migrasi ke Postgres dijaga (tidak ada fitur SQLite-spesifik di query).
- **Scheduler**: APScheduler dalam proses API (job per jam/harian) + endpoint manual `POST /jobs/{name}/run`.
- **LLM**: abstraksi `LLMProvider` (Anthropic pertama; OpenAI dan self-hosted/Ollama sebagai implementasi tambahan). Routing per tier: `light` (capture/hygiene), `heavy` (deal/forecast/brief), `interactive` (ask).
- **MCP**: MCP Python SDK (FastMCP), transport Streamable HTTP, OAuth 2.1 per pengguna.
- **Web**: Vite + React + TypeScript, tanpa UI framework berat; design tokens diambil dari mockup. Dibundel statis dan disajikan FastAPI.
- **Integrasi**: Odoo XML-RPC (`xmlrpc.client`, tanpa lib pihak ketiga), Gmail/Calendar API (google-api-python-client), WhatsApp Cloud API (webhook + REST), Truecaller Business API, Basecamp API (opsional).
- **Deploy**: Docker Compose (api, web statis via Caddy, scheduler = proses api), backup SQLite harian ke Google Drive.
- **Auth**: pengguna internal dari daftar di DB (seed dari Odoo `res.users`), sesi cookie + API key untuk mesin, OAuth 2.1 untuk klien MCP.

## Konsekuensi
SQLite membatasi tulis paralel — job ditulis batch dan singkat. React menambah build step, tapi mockup sudah kaya interaksi (3D, chat) sehingga vanilla akan lebih mahal dirawat. Semua pilihan bisa diganti tanpa menyentuh domain (`packages/core` tidak boleh mengimpor FastAPI/React).
