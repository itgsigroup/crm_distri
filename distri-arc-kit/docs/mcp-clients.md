# Klien MCP — menyambungkan Claude Desktop, ChatGPT, dan agent lain ke Distri ARC

Distri ARC menyajikan MCP server (Streamable HTTP) di **`<PUBLIC_URL>/mcp`** — produksi `https://distri.gsi.co.id/mcp`,
dev `http://localhost:8080/mcp`. Klien MCP bisa **membaca**, **menganalisis**, dan **menjalankan Orchestrator**.
Klien MCP **tidak pernah** memutuskan atau mengirim ke dealer: `actions.decide` selalu menjawab `human_only`, dan
`allow_send` terkunci di kode.

## 1. Buat token
Hanya CEO. Token ditampilkan **sekali**; yang disimpan hanya hash argon2id.

- Aplikasi: Pengaturan → Koneksi AI → *MCP · Distri ARC sebagai server* → **Buat token**, atau Orchestrator → *Klien & token*.
- CLI: `bin/arc ctl mcp-token --name "Claude Desktop Sam" --scopes read,analyze,orchestrate`

| Scope | Tool |
|---|---|
| `read` | `dealer.list`, `dealer.get`, `segmen.list`, `jadwal.due`, `jadwal.lewat`, `kredit.check`, `stok.aging`, `chat.thread`, `kpi.utama`, `cycles.recent`, `proposals.list` |
| `analyze` | `analisis.dealer`, `analisis.segmen`, `analisis.kas`, `analisis.stok` |
| `orchestrate` | `orchestrator.run`, `orchestrator.reanalyze`, `orchestrator.agent.run`, `orchestrator.plan`, `orchestrator.plan.update`, `orchestrator.input.get`, `orchestrator.submit`, `orchestrator.status` |
| — | `actions.decide` → selalu `human_only` |

Resources: `arc://glossary`, `arc://policies`, `arc://dealer/{id}`. Prompts: `morning_brief`, `dealer_review {dealer}`.

Batas: 60 panggilan/menit per klien; siklus dari MCP maksimal `mcp.permissions.max_cycles_per_hour` (default 6) per
klien per jam. Setiap panggilan tercatat di `mcp_calls` + `audit_log` dan tampil di layar Orchestrator.

## 2. Claude Desktop
`~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) atau `%APPDATA%\Claude\claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "distri-arc": {
      "url": "https://distri.gsi.co.id/mcp",
      "headers": { "Authorization": "Bearer arc_…" }
    }
  }
}
```

Versi Claude Desktop yang hanya mendukung server stdio dapat memakai mode stdio di mesin yang punya akses database:

```json
{
  "mcpServers": {
    "distri-arc": {
      "command": "/opt/distri-arc/bin/arc",
      "args": ["ctl", "mcp-stdio"],
      "env": { "ARC_MCP_TOKEN": "arc_…", "DATABASE_URL": "postgres://…" }
    }
  }
}
```

Coba: *"Dealer mana milik Dewi yang jadwal ordernya minggu ini?"* (→ `jadwal.due`), *"Analisis ulang Mitra Jaya"*
(→ `orchestrator.reanalyze {scope:"dealer:mitra"}`; siklus `trigger=mcp` muncul di Riwayat analisis).

## 3. ChatGPT (connector / MCP)
Settings → Connectors → *Add custom connector* → URL `https://distri.gsi.co.id/mcp`, autentikasi **Bearer token** →
tempel token `arc_…`. Beri scope `read` (dan `analyze`) untuk tim sales; `orchestrate` hanya untuk pengguna yang
boleh memicu siklus.

## 4. curl (Streamable HTTP)
```bash
TOKEN=arc_…
curl -s https://distri.gsi.co.id/mcp -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}' -i
# ambil header Mcp-Session-Id dari jawaban, lalu:
curl -s https://distri.gsi.co.id/mcp -H "Authorization: Bearer $TOKEN" -H "Mcp-Session-Id: <id>" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"jadwal.due","arguments":{"sales":"Dewi"}}}'
```

Lebih praktis: `go run ./tools/mcpcheck -url http://localhost:8080/mcp -token "$TOKEN" jadwal.due '{"sales":"Dewi"}'`.

## 5. Model eksternal sebagai mesin analisis (jalur `mcp`)
Pengaturan → Koneksi AI → *Analisis via* **MCP** (atau `PUT /api/policies/llm {"mode":"mcp"}`). Setiap siklus lalu
menerbitkan Input tiap agen dan menunggu (maks 10 menit) klien yang:

1. `orchestrator.status` → `cycle.id` siklus yang sedang di tahap Analisis;
2. `orchestrator.input.get {cycle_id, agent}` → Input yang **dimasking** (`<PIC_n>`, `<NO_n>`) berisi dealer, sinyal, dan
   `candidates` hasil hitungan Go (angka otoritatif);
3. `orchestrator.submit {cycle_id, agent, proposals:[…]}` → divalidasi: `kind` milik agen, `signal_ids` hanya dari Input,
   dealer dikenal, confidence 0–1. Proposal dari MCP **tidak pernah otonom**.

Agen yang tidak dijawab memakai template Go dan tahap ditandai `partial`. Dalam mode **Keduanya**, klien boleh
menambah proposal untuk siklus yang sudah selesai (`mode: stored`).
