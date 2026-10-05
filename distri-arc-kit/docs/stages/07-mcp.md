# Stage 07 — MCP server: baca · analisis · orkestrasi; MCP sebagai orchestrator; Koneksi AI
**Baca**: `06-mcp.md` (seluruhnya), ADR 0004, `09` (mcp.permissions), mockup: panel "MCP sebagai orchestrator" dan Pengaturan → Koneksi AI.

## Tujuan
Claude Desktop/ChatGPT bisa membaca, menganalisis, dan menjalankan Orchestrator lewat MCP dengan token ber-scope, audit, dan rate limit; model eksternal bisa menjadi mesin analisis lewat `input.get`/`submit`; tidak ada jalur kirim.

## Deliverables
1. `internal/mcp`: server Streamable HTTP di `/mcp` (SDK Go resmi) + `arc ctl mcp-stdio`; auth bearer → `mcp_clients` (argon2id), scope check per tool, rate limit (60/mnt; `orchestrator.*` sesuai `max_cycles_per_hour`), `mcp_calls` + `audit_log`.
2. Tool lengkap sesuai 06 (baca, analisis, orkestrasi), resources (`arc://glossary`, `arc://policies`, `arc://dealer/{id}`), prompts (`morning_brief`, `dealer_review`). `actions.decide` → `human_only`.
3. Jalur `mcp` di Orchestrator: `orchestrator.input.get {cycle_id, agent}` mengembalikan `Input` masked; `orchestrator.submit` memvalidasi schema + provenance dan menyimpan proposal `proposed` ke siklus yang sama; `policy.llm.routing = mcp` membuat tahap Analisis menunggu submit maks 10 menit (lalu `partial`).
4. `arc ctl mcp-token --name --scopes` (tampilkan token sekali) · API `/mcp/clients`, `/mcp/calls`, `/policies/mcp`, `/policies/llm`.
5. `docs/mcp-clients.md`: konfigurasi Claude Desktop (`mcpServers`), ChatGPT connector, `curl` contoh; diuji nyata dengan Claude Desktop Sam (OPEN-QUESTIONS bila belum).
6. Frontend: panel **MCP sebagai orchestrator** di layar Orchestrator (kebijakan 3 saklar — `allow_send` terkunci, log panggilan, daftar tool), **Pengaturan → Koneksi AI** (API AI / MCP / Keduanya, kartu koneksi + status, endpoint + salin, aturan), daftar klien MCP + buat token.
7. Uji 06 (semua) + uji rate limit + uji `routing=mcp` dengan klien uji yang memanggil `input.get`→`submit`.

## Acceptance
- Dari Claude Desktop (atau klien uji SDK): `jadwal.due {sales:"Dewi"}` → 3 dealer; `orchestrator.reanalyze {scope:"dealer:<mitra>"}` → siklus `trigger=mcp` tampil di Riwayat dan log panggilan di UI ≤ 3 dtk.
- Token scope `read` memanggil `orchestrator.run` → `forbidden`, tercatat.
- `actions.decide` → `human_only`. `PUT /policies/mcp {allow_send:true}` → `400`.
- `make check` hijau.

Commit: `feat(stage-07): mcp server (read/analyze/orchestrate), mcp sebagai orchestrator, koneksi ai`
