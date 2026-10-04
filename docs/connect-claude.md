# Menghubungkan Claude ke ARC (MCP)

ARC menyediakan server MCP (Streamable HTTP, protokol 2025-06-18) di `https://<domain-arc>/mcp`. Claude bisa membaca akun, deal, chat, kas, dan **mengusulkan** tindakan. Claude **tidak pernah bisa memutuskan**: `arc.actions.decide` selalu ditolak untuk token mesin/klien AI — keputusan hanya dari pengguna manusia di web ARC.

## Pilihan A — claude.ai / Claude Desktop sebagai connector (OAuth, disarankan)
1. claude.ai → Settings → Connectors → **Add custom connector**.
2. Nama: `ARC`, URL: `https://<domain-arc>/mcp`.
3. Klik **Connect** → jendela login ARC terbuka → login dengan akun ARC Anda → **Izinkan**.
   ARC memakai OAuth 2.1 (authorization code + PKCE S256, dynamic client registration). Token mewarisi **peran dan cabang** Anda: sales Semarang hanya melihat akun Semarang.
4. Di percakapan, aktifkan connector ARC lalu tanya, mis. *"Brief akun RSUD Kota Yogyakarta"* atau *"Deal mana yang berisiko minggu ini?"*.

## Pilihan B — API key (Claude Desktop config / Claude Code)
1. Web ARC → **Pengaturan → MCP & API** → **Buat API key** → pilih scope (`read`, `propose`). Key `arc_live_…` tampil **sekali** — simpan.
2. Claude Desktop (`claude_desktop_config.json`):
   ```json
   {
     "mcpServers": {
       "arc": {
         "url": "https://<domain-arc>/mcp",
         "headers": { "Authorization": "Bearer arc_live_xxxxxxxx" }
       }
     }
   }
   ```
3. Claude Code:
   ```bash
   claude mcp add --transport http arc https://<domain-arc>/mcp --header "Authorization: Bearer arc_live_xxxxxxxx"
   ```

## Tools
Nama di wire memakai garis bawah (`arc_accounts_brief`), ditampilkan sebagai `arc.accounts.brief`.

| Grup | Tools | Sifat |
|---|---|---|
| Akun | `arc.accounts.list`, `arc.accounts.brief`, `arc.accounts.memory.append`, `arc.commitments.list`, `arc.commitments.create` | baca / tulis tercatat audit |
| Deal | `arc.deals.list`, `arc.deals.forecast`, `arc.deals.probability.propose`, `arc.tenders.list`, `arc.prospects.list`, `arc.prospects.brief` | baca / usulan |
| Chat | `arc.chat.threads`, `arc.chat.read`, `arc.chat.reply.draft`, `arc.network.graph` | baca / draf → antrean approval |
| Kas | `arc.cash.l2c`, `arc.cash.aging`, `arc.cash.forecast`, `arc.cash.reminder.draft` | baca / draf → antrean approval |
| Tindakan | `arc.actions.list`, `arc.actions.propose`, `arc.actions.decide` (**human-only, ditolak**), `arc.policy.get`, `arc.policy.set` (CEO) | |

Grup tools bisa dimatikan per grup di **Pengaturan → AI & model**. Chat pribadi antar karyawan tidak pernah tersedia lewat MCP.

## Uji cepat
```bash
make mcp-inspect          # MCP Inspector ke http://localhost:8000/mcp
```
Cek: tools terdaftar · `arc_accounts_brief {"account_id":"rsud"}` mengembalikan brief + evidence · `arc_actions_decide` → error "human-only".
