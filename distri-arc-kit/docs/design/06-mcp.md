# 06 · MCP — Distri ARC sebagai server (dan sebagai orchestrator lewat klien)

## Transport & auth
- Endpoint `https://<host>/mcp` — **Streamable HTTP** (MCP spec 2025-06), dilayani oleh `cmd/api` dengan SDK Go resmi. Mode `stdio` untuk `arc ctl mcp-stdio` (dev lokal).
- Auth: header `Authorization: Bearer <token>`; token dibuat `arc ctl mcp-token --name "Claude Desktop Sam" --scopes read,analyze,orchestrate`; disimpan hash (argon2id) di `mcp_clients`. Scope: `read` · `analyze` · `orchestrate` · (`decide` **tidak pernah diberikan** ke klien; endpoint keputusan hanya di API dengan sesi manusia).
- Rate limit per klien: 60 panggilan/menit; `orchestrator.*` 6/jam.
- Semua panggilan ke `mcp_calls` + `audit_log`.

## Tool
### Baca (`read`)
| Tool | Args | Hasil |
|---|---|---|
| `dealer.list` | `{status?, segment?, sales?, q?, limit?}` | daftar `v_dealer_board` |
| `dealer.get` | `{id|name}` | dealer lengkap: metrics, 5 komponen skor, PIC, memo, komitmen, 10 sinyal terakhir |
| `segmen.list` | `{}` | 4 segmen: dealer, omzet/bln, % omzet, perpindahan 3 bulan |
| `jadwal.due` | `{days?=7, sales?}` | dealer jadwal order + rekomendasi order |
| `jadwal.lewat` | `{sales?}` | dealer lewat jadwal + akar terduga |
| `kredit.check` | `{dealer_id, amount?}` | sisa limit, state, simulasi rilis |
| `stok.aging` | `{branch?, min_days?}` | stok menua + kandidat push |
| `chat.thread` | `{dealer_id, limit?=30}` | pesan (PII dimasking sesuai scope) |
| `kpi.utama` | `{branch?}` | order tepat jadwal, DSO, perputaran stok |
| `cycles.recent` | `{limit?=10}` | riwayat siklus + counters |
| `proposals.list` | `{status?='proposed'}` | antrean keputusan (baca saja) |

### Analisis (`analyze`)
| Tool | Keterangan |
|---|---|
| `analisis.dealer` `{dealer_id}` | Orchestrator menyiapkan `Input` scope dealer dan menjalankan agen dengan jalur yang aktif; kembali ringkasan + proposal baru (status `proposed`) |
| `analisis.segmen` `{segment}` | ringkasan segmen + 3 tindakan |
| `analisis.kas` `{days?=30}` | prediksi kas masuk tertimbang pola bayar + 2 tindakan yang paling menaikkan |
| `analisis.stok` `{branch?}` | kandidat push per SKU |

### Orkestrasi (`orchestrate`) — MCP sebagai orchestrator
| Tool | Keterangan |
|---|---|
| `orchestrator.run` `{}` | siklus penuh (`trigger='mcp'`), tunduk advisory lock; kembali `cycle_id` + counters |
| `orchestrator.reanalyze` `{scope}` | scope `all | screen:<orbit|segmen|stock|credit> | dealer:<id> | agent:<name>` |
| `orchestrator.plan` `{date?}` | Rencana hari ini (plan_items) |
| `orchestrator.plan.update` `{item_id, action:'move'|'skip'|'add', ...}` | **membuat proposal perubahan rencana** berstatus `proposed` (butuh approve manusia); tidak mengubah rencana langsung |
| `orchestrator.agent.run` `{agent, dealer_id?}` | menjalankan satu agen (scope agent) |
| `orchestrator.submit` `{cycle_id, agent, proposals:[...]}` | jalur **MCP sebagai mesin analisis**: klien MCP (model eksternal) mengembalikan proposal untuk `Input` yang diminta lewat `orchestrator.input.get {cycle_id, agent}`; Orchestrator memvalidasi schema + provenance (signal_ids harus ada dalam Input) sebelum menerima |
| `orchestrator.status` `{}` | status, stage, via, berikutnya |

### Keputusan
`actions.decide` **tidak ada** di MCP. Klien yang mencoba mendapat `error: human_only`.

## Kebijakan MCP (`policy.mcp.permissions`, diedit di layar Orchestrator)
```json
{ "allow_reanalyze": true, "allow_plan_update_proposal": true, "allow_send": false, "mask_pii_in_read": true, "max_cycles_per_hour": 6 }
```
`allow_send` tidak bisa diubah dari UI (terkunci di kode).

## Resources & prompts
- Resource `arc://glossary` (01-glossary.md), `arc://policies` (JSON kebijakan aktif), `arc://dealer/{id}` (markdown ringkas).
- Prompt `morning_brief` (ringkasan pagi dari `cycles.latest`), `dealer_review {dealer}`.

## Konfigurasi klien
`docs/mcp-clients.md` (dibuat Stage 07) berisi JSON untuk Claude Desktop (`mcpServers.distri-arc` dengan `url` + header), ChatGPT (connector), dan contoh `curl` Streamable HTTP.

## Uji wajib (`internal/mcp`)
- Token tanpa scope `orchestrate` memanggil `orchestrator.run` → `error: forbidden`, tercatat di `mcp_calls`.
- `orchestrator.reanalyze {scope:'dealer:mitra'}` → `cycles` baru `trigger='mcp'`, `scope='dealer:<id>'`, proposal baru `proposed`.
- `orchestrator.submit` dengan `signal_ids` di luar Input → ditolak.
- `actions.decide` → `human_only`.
- `chat.thread` memasking nomor WA bila `mask_pii_in_read`.
