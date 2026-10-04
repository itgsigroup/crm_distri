# Stage 06 — MCP server, AI API, Ask

## Tujuan
ARC bisa ditanya dan dioperasikan dari Claude/ChatGPT lewat MCP, dari sistem lain lewat REST, dan dari UI lewat Ask — dengan otorisasi yang mewarisi hak pengguna.

## Baca dulu
`04` (keamanan, human-only), `01` (model tindakan), `05-ui-spec` (Pengaturan AI & MCP, Ask), mockup daftar tool/endpoint.

## Kerjakan
1. `packages/mcp/server.py` (FastMCP, Streamable HTTP `/mcp`): tool sesuai mockup — `arc.accounts.list/brief/memory.append`, `arc.commitments.list/create`, `arc.deals.list/forecast/probability.propose`, `arc.chat.threads/read/reply.draft`, `arc.network.graph`, `arc.actions.list/propose/decide`, `arc.policy.get/set`, `arc.prospects.list/brief` (+ `arc.cash.*`, `arc.tenders.*` menyusul di tahap 11–12). Tool write hanya membuat Action `proposed`; `decide` menolak token mesin; `policy.set` hanya CEO. Toggle per group.
2. OAuth 2.1 (auth code + PKCE) untuk klien MCP; token mewarisi user/role/cabang; resource metadata sesuai spesifikasi MCP.
3. REST `/api/v1`: `POST /ask`, `GET /accounts/{id}/brief`, `GET /signals`, `GET /actions`, `POST /actions/{id}/decision` (scope `human`), `POST /events`, webhook keluar (`action.proposed`, `commitment.overdue`, `signal.detected`) HMAC; OpenAPI 3.1 (Custom GPT).
4. **Ask** (interactive tier): retrieval sederhana (akun/orang/komitmen/sinyal/opportunity relevan + 20 interaksi terakhir) → jawaban + evidence cards; pertanyaan kontekstual per layar (`GET /ask/suggestions?screen=`); UI Ask & dropdown Tanya ARC aktif.
5. API key: create/revoke/scope/rate limit/last used; audit tiap panggilan.
6. Dokumentasi `docs/connect-claude.md`, `docs/connect-chatgpt.md` + tampil di Pengaturan → MCP & API (snippet, copy). `make mcp-inspect`.

## Acceptance criteria
- MCP Inspector: tools terdaftar; `arc.accounts.brief("rsud")` mengembalikan brief + evidence; `arc.actions.decide` via token mesin → ditolak.
- Token sales Semarang tidak bisa membaca akun Yogyakarta (test).
- `POST /ask "Deal mana yang berisiko?"` → jawaban dengan ≥ 2 evidence id dari seed.
- Webhook keluar terkirim dengan signature valid saat Action baru.
