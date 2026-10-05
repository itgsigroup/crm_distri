# ADR 0009 — Implementasi MCP server: token, cara siklus berjalan, jalur `mcp`
**Status**: diterima · 2026-10-05 · melengkapi ADR 0004 dan `06-mcp.md`

## Keputusan
1. **SDK Go resmi** `github.com/modelcontextprotocol/go-sdk` v1.8 (`mcp`, `auth`): Streamable HTTP di `/mcp` pada proses
   API (bukan di bawah `/api`, karena memakai bearer token, bukan sesi pengguna) dan `arc ctl mcp-stdio`.
2. **Token** `arc_<client-id-hex>_<secret>`: id di depan agar verifikasi tidak perlu memindai tabel; rahasia disimpan
   sebagai hash argon2id (salt acak, t=2, m=19 MiB). Verifikasi di-cache 1 menit per proses; mencabut token
   mengosongkan cache. Token hanya dibuat CEO (API) atau lewat CLI server dan ditampilkan sekali.
3. **Satu jalur orkestrasi**: tool `orchestrator.*`/`analisis.dealer` menulis siklus `queued` (`trigger=mcp`,
   `requested_by` = nama klien) + job `cycle.run` dalam satu transaksi — sama dengan tombol *Analisis ulang* — lalu
   menunggu ≤ 25 dtk agar jawaban klien berisi hasil. Batas `max_cycles_per_hour` dihitung dari tabel `cycles`.
4. **Jalur `mcp`** (`llm.routing.mode = mcp`): tahap Analisis menerbitkan Input per agen ke `cycle_inputs` (dimasking
   per nilai string; pemetaan placeholder disimpan di server, tidak pernah dikirim) dan menunggu `orchestrator.submit`
   maks 10 menit; agen tanpa jawaban memakai template Go (`partial`). Submit divalidasi: kind agen, `signal_ids` ⊆ Input,
   dealer dikenal, lalu di-unmask per string. Proposal dari MCP selalu `approve`.
5. **Tidak ada jalur kirim**: `actions.decide` terdaftar agar klien mendapat `human_only` yang tercatat; `allow_send`
   dipaksa `false` saat dibaca dan `PUT /policies/mcp {allow_send:true}` → `400`.
6. **Glosarium** di-embed dari salinan `internal/mcp/resources/glossary.md`; test memastikan identik dengan
   `docs/design/01-glossary.md`.
