# Stage 00 — Bootstrap repo, ADR, tooling

## Tujuan
Repo yang bisa dijalankan dalam 5 menit: monorepo (api, web, wa-bridge), tooling, seed/fixture dari mockup, Docker Compose skeleton.

## Baca dulu
`CLAUDE.md`, `docs/adr/0001-stack.md`, `docs/adr/0002-whatsapp-backend.md`, `docs/knowledge/00`, `01`, `05`.

## Kerjakan
1. Struktur: `apps/api`, `apps/web`, `apps/wa-bridge`, `packages/core`, `packages/connectors`, `packages/mcp`, `infra`, `tests`, `docs`. Python workspace `uv` (fallback pip-tools). Web: `npm create vite@latest apps/web -- --template react-ts`. wa-bridge: `npm init` + TypeScript + `tsx`, dependensi Baileys **belum dipasang** (tahap 02).
2. `Makefile`: `make dev` (api reload + vite + wa-bridge dev), `make test`, `make lint` (ruff, mypy strict `packages/core`, eslint, tsc), `make seed`, `make db-upgrade`, `make fmt`.
3. FastAPI minimal: `GET /health` → `{status, version, stage}` (dari `.arc/progress.json`). Logging JSON.
4. Web minimal: halaman "ARC" memuat `tokens.css` (salin dari mockup) + hasil `/health`; light/dark.
5. wa-bridge minimal: `GET /health` di :3001, membaca `.env` (`API_URL`, `BRIDGE_SECRET`).
6. `infra/docker-compose.yml` (api, wa-bridge, caddy), `Caddyfile`, Dockerfile multi-stage.
7. `.env.example` lengkap untuk semua tahap (Anthropic, OpenAI, WA bridge secret, WA Cloud, Truecaller, Odoo, Google, Basecamp, SMTP) dengan komentar.
8. Pre-commit: ruff, eslint, gitleaks.
9. `scripts/extract_mockup_fixtures.py`: ekstrak konstanta `DEALS`, `WON`, `LEADS`, `CONTACTS`, `EDGES_M`, `CHATS`, `INBOUND`, `INTERNAL`, `L2C`, `ACTIONS` dari `reference/arc-crm-mockup.html` → `tests/fixtures/*.json`.
10. README: cara jalan + `/stage`.

## Acceptance criteria
- `make dev` → :8000 `/health` OK, :5173 menampilkan status, :3001 `/health` OK.
- `make test` hijau (health + loader fixture: ≥ 8 deal, ≥ 25 edge, ≥ 8 chat thread, ≥ 4 inbound). `make lint` bersih. Compose valid. gitleaks lolos.
