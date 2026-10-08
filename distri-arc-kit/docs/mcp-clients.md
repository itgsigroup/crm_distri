# Menghubungkan Claude ke Distri ARC (MCP)

Endpoint: `https://crm-distri.gsiindo.id/mcp` (Streamable HTTP). Semua langkah juga ada di aplikasi: **sidebar → MCP Claude**.
Claude hanya membaca dan mengusulkan; setujui/tolak dan pengiriman ke dealer tetap di aplikasi (ADR 0004, 0021).

## claude.ai dan Claude Desktop (disarankan, login OAuth)
1. Settings → Connectors → **Add custom connector** (Team/Enterprise: owner di Admin settings → Connectors).
2. Name `Distri ARC`, URL `https://crm-distri.gsiindo.id/mcp` → Add → **Connect**.
3. Halaman Distri ARC terbuka: login (pengguna dengan menu MCP Claude dan cakupan semua data) → **Izinkan**.
4. Di percakapan, aktifkan Distri ARC dari menu alat, lalu mulai dengan "panggil data_ringkasan …".

## Claude Code
```
claude mcp add --transport http distri-arc https://crm-distri.gsiindo.id/mcp
```
lalu `/mcp` → distri-arc → Authenticate → Izinkan. Dengan token manual:
`claude mcp add --transport http distri-arc https://crm-distri.gsiindo.id/mcp --header "Authorization: Bearer <TOKEN>"`.

## Token manual (agent sendiri, Claude API, ChatGPT)
Dibuat CEO di MCP Claude → Token manual (ditampilkan sekali). Claude Desktop tanpa OAuth: `npx -y mcp-remote <url>
--header "Authorization:Bearer ${ARC_TOKEN}"` di `claude_desktop_config.json`. Claude API: `mcp_servers: [{type: "url",
url, name: "distri-arc", authorization_token}]`. CLI: `arc ctl mcp-token --name … --scopes read,analyze`.

## Tool
Baca: `data_ringkasan` (mulai dari sini), `dealer_list` (filter cabang/jenis/status/segmen/sales, urut, halaman),
`dealer_get`, `segmen_list`, `jadwal_due`, `jadwal_lewat`, `kredit_check`, `stok_aging`, `chat_thread`, `kpi_utama`,
`penjualan_bulanan`, `produk_terlaris`, `piutang_ringkas`, `cycles_recent`, `proposals_list`. Analisis: `analisis_dealer`,
`analisis_segmen`, `analisis_kas`, `analisis_stok`. Orchestrator: `orchestrator_*`. `actions_decide` selalu human_only.
Daftar dihalaman: `limit` (default 50, maks 500) dan `offset`; jawaban memuat `total` dan `next_offset`.

## Analisis terjadwal (cron)
Di halaman **MCP Claude → Analisis terjadwal**: buat jadwal (preset atau cron `menit jam tanggal bulan hari`, WIB,
jarak minimal 15 menit), tulis prompt, pilih izin tool dan batas langkah. Worker menjalankannya; laporan tersimpan di
**Laporan**. CEO mengisi kunci Claude API (console.anthropic.com → API keys) di **Atur mesin**, memilih model, dan
menetapkan anggaran harian. Tanpa kunci, laporan berisi angka dari tool MCP (mode template).

## Uji cepat
`go run ./tools/mcpcheck -url https://crm-distri.gsiindo.id/mcp -token <TOKEN>`.
