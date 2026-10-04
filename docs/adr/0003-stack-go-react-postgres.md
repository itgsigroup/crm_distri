# ADR 0003 — Stack diganti: Go + React + PostgreSQL
Status: diterima (tahap 00, 2026-10-04); baris WhatsApp bridge digantikan ADR 0004 (kembali ke Baileys). Menggantikan bagian stack di ADR 0001 dan bagian implementasi bridge di ADR 0002. Prinsip produk, domain, dan perilaku tidak berubah.

## Konteks
Pemilik proyek meminta secara eksplisit: **backend Golang, frontend React, database PostgreSQL**, dengan fungsi dan tampilan tetap sama persis dengan build kit (prompt tahap + mockup). ADR 0001 memilih Python/FastAPI/SQLite; ADR 0002 memilih Node + Baileys untuk `wa-bridge`. CLAUDE.md §3 mewajibkan perubahan stack lewat ADR baru dengan persetujuan Sam — persetujuan itu adalah permintaan ini.

## Keputusan
| Area | ADR 0001/0002 | Sekarang | Catatan |
|---|---|---|---|
| Backend API | Python 3.12 · FastAPI | **Go 1.26** · `net/http` stdlib (ServeMux dengan pola method) | Satu biner `arc` (serve/migrate/seed/reset/brief/job/eval). |
| Database | SQLite (WAL) + SQLAlchemy + Alembic | **PostgreSQL 17** · `pgx/v5` | Migrasi SQL **tertanam** (`packages/core/storage/migrations/*.sql`, tabel `schema_migrations`) menggantikan Alembic; tetap berurutan, tidak ada `create_all`. Audit log append-only dijaga trigger. |
| Scheduler | APScheduler | Scheduler dalam proses (goroutine) | Job per jam, harian 06.30, brief 06.45/16.00 WIB; tetap ada `POST /jobs/{name}/run` dan `arc job <name>`. |
| Validasi | Pydantic v2 | struct Go + JSON Schema untuk output LLM | Structured output Anthropic (`output_config.format`). |
| LLM | Anthropic SDK Python | `anthropic-sdk-go` | Abstraksi provider, tier light/heavy/interactive, masking PII, pencatatan biaya tidak berubah. FakeProvider deterministik bila tanpa kunci. |
| MCP | MCP Python SDK (FastMCP) | Implementasi Streamable HTTP JSON-RPC sendiri (`packages/mcp`) | Protokol 2025-06-18, OAuth 2.1 (PKCE S256, dynamic client registration, protected-resource metadata). Nama tool di wire `arc_accounts_list` (klien menolak titik), tampil `arc.accounts.list`. |
| WhatsApp bridge | Node 20 + Baileys | ~~Go + whatsmeow~~ → **Node + Baileys** lagi (ADR 0004) | Fitur sama: multi-sesi QR, riwayat N hari, grup + anggota, profil, kirim hanya untuk Action approved (dicek ulang ke API), ≤ 20/jam/sesi, jeda acak 2–6 dtk, antrean file saat API mati. |
| Web | Vite + React + TS | Sama | CSS mockup disalin apa adanya; Three.js untuk Peta 3D. |
| Lint | ruff + mypy strict | `go vet` + `gofmt` · `oxlint` + `tsc` | "mypy strict" (tahap 01) dipenuhi oleh sistem tipe Go + `go vet`. |
| Aturan NBA | YAML | JSON (`packages/core/agents/rules/nba.json`, embed) | Menghindari dependensi YAML; tetap table-driven. |
| Test | pytest · vitest | `go test` (integrasi ke DB `arc_test`) · vitest · Playwright | |
| Deploy | Compose + Caddy, backup SQLite ke Drive | Compose (postgres, api, wa-bridge, caddy) + `pg_dump` harian | `scripts/backup.sh` / `restore.sh`. |

## Penyimpanan auth-state WhatsApp
Bridge menyimpan kunci perangkat (auth state Baileys) di PostgreSQL skema `wa_bridge` (terpisah dari `public`, jadi `arc reset` tidak memutus HP yang sudah tertaut). Kunci ini setara akses ke akun WhatsApp: database harus di disk terenkripsi (LUKS di Mini PC / volume terenkripsi di VPS), akses DB hanya dari jaringan compose, dan dump backup diperlakukan rahasia. Token OAuth Google dienkripsi AES-256-GCM dengan `ARC_ENCRYPTION_KEY`.

## Konsekuensi
- Domain (`packages/core`) tetap bebas dari HTTP/UI; konektor di `packages/connectors`, persis struktur CLAUDE.md §5 dengan Go menggantikan Python.
- PostgreSQL menghapus batasan tulis paralel SQLite; butuh satu layanan tambahan di compose.
- Tim perawat perlu Go, bukan Python. Biner tunggal tanpa runtime memudahkan deploy di Mini PC.
- Perintah `make dev/test/lint/seed` tetap ada dengan arti yang sama.
