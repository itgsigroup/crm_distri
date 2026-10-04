# ARC — Agentic Relationship Core (GSI)
CRM AI-native untuk PT Gosyen Solusi Indonesia: **AI mengisi database dan mengusulkan, manusia memutuskan.** WhatsApp, email, kalender, dan Odoo dibaca; agen mengekstrak komitmen, sinyal, health deal, forecast, prediksi kas; setiap tindakan keluar menunggu persetujuan manusia.

Stack (ADR 0003): **Go 1.26** (API, scheduler, MCP, wa-bridge whatsmeow) · **PostgreSQL 17** · **React + Vite + TypeScript** (UI sesuai `reference/arc-crm-mockup.html`).

## Menjalankan (development)
Prasyarat: Go ≥ 1.26, Node ≥ 20, PostgreSQL ≥ 15.
```bash
cp .env.example .env                 # isi DATABASE_URL; kunci API boleh kosong (mock)
createdb arc && createdb arc_test
(cd apps/web && npm ci)
make db-upgrade && make seed         # data fixture mockup (idempoten)
make dev                             # API :8000 · wa-bridge :3001 · web :5173
```
Buka http://localhost:5173 — login `sam@gsi.co.id` (CEO) atau `andi@gsi.co.id` (sales Semarang), sandi `arc12345` (hanya data demo).

Dengan `ARC_CLOCK_ANCHOR=2026-09-28T17:02:00+07:00` jam aplikasi dimulai pada tanggal mockup sehingga angka seed sama dengan mockup (forecast Rp 4,3/5,9/11,9 M, kas Rp 2,4 M, health RSUD 43). Kosongkan untuk jam asli.

## Perintah
| Perintah | Fungsi |
|---|---|
| `make dev` | API + bridge + web sekaligus |
| `make test` | `go test` (integrasi ke `arc_test`) + test bridge + vitest |
| `make lint` | `go vet`, `gofmt`, `oxlint`, `tsc` |
| `make seed` / `make reset` | muat fixture / hapus semua lalu muat ulang (ditolak di production) |
| `make eval` | evaluasi ekstraksi → `docs/eval/capture-<provider>.md` |
| `make e2e` | Playwright smoke (butuh `make dev` + seed) |
| `make build` | `bin/arc`, `bin/wa-bridge`, `apps/web/dist` |
| `make mcp-inspect` | MCP Inspector ke `/mcp` |
| `make backup` / `make restore FILE=…` | pg_dump / pg_restore |
| `make compose-up` | stack produksi (postgres, api, wa-bridge, caddy) |

CLI: `bin/arc serve | migrate | seed | reset | brief [--send] | job <nama> | eval`.

## Struktur
```
apps/api          Go HTTP API, scheduler, executor, MCP/OAuth, webhook (cmd/arc)
apps/web          React UI (CSS mockup apa adanya, Three.js untuk Peta 3D)
apps/wa-bridge    sidecar WhatsApp (whatsmeow, modul Go terpisah)
packages/core     domain, storage+migrasi SQL, health, insights, actions, agents, llm, prompts
packages/connectors  whatsapp, odoo (XML-RPC), google (Gmail/Calendar), identity, notify
packages/mcp      definisi tools MCP
infra/            docker-compose, Dockerfile, Caddyfile
tests/fixtures    data mockup (JSON, .eml, kalender) · tests/eval kasus ekstraksi
docs/             knowledge, stages, ADR, runbook, go-live, koneksi Claude/ChatGPT
```

## Mock vs asli
Tanpa kredensial, konektor memakai mock yang realistis dan `/health` menampilkan `mocks`: LLM (FakeProvider deterministik), Odoo (FakeOdoo), Google (12 email + 3 event fixture), Truecaller/web search (fixture), notifier (dicatat di tabel `notifications`), WhatsApp (FakeTransport; bridge asli siap dipakai dengan scan QR). Daftar kredensial yang dibutuhkan: `docs/OPEN-QUESTIONS.md`.

## Dokumen
- Aturan pengembangan: `CLAUDE.md` · status tahap: `.arc/progress.json` · `CHANGELOG.md`
- Menghubungkan AI: `docs/connect-claude.md`, `docs/connect-chatgpt.md`
- Operasi: `docs/runbook.md`, `docs/go-live-checklist.md`, `docs/testing/whatsapp.md`
- Pelatihan sales: `docs/training/sales-1-halaman.md`

## Bertahap dengan Claude Code
`/stage status` menampilkan posisi; `/stage redo N` mengulang tahap dari acceptance criteria-nya. Semua tahap 00–13 sudah dikerjakan (beberapa `done-with-mocks` sampai kredensial diisi).
