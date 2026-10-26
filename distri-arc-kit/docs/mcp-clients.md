# Menghubungkan Claude ke Distri ARC (MCP)

Endpoint: `https://crm-distri.gsiindo.id/mcp` (Streamable HTTP). Semua langkah juga ada di aplikasi: **sidebar → MCP Claude**.
Claude hanya membaca dan mengusulkan; setujui/tolak dan pengiriman ke dealer tetap di aplikasi (ADR 0004, 0021).

## Hermes Agent pada device lain
Hermes menjadi klien MCP dan memanggil tool CRM melalui HTTPS; Hermes tidak perlu dipasang di server CRM.

1. Buka **MCP Claude → Token manual** di CRM, buat token bernama `Hermes Agent`, dan pilih izin `read` + `analyze`.
2. Salin blok YAML **Hermes Agent** yang ditampilkan bersama token. URL dan header Bearer sudah diisi oleh CRM.
3. Pada device Hermes, gabungkan konfigurasi itu ke `~/.hermes/config.yaml` di bawah `mcp_servers`. Jangan mengganti konfigurasi MCP lain yang sudah ada. Batasi akses file token dengan `chmod 600 ~/.hermes/config.yaml`.
4. Lindungi file konfigurasi karena berisi token, restart Hermes, lalu jalankan `hermes mcp test distri_crm`.
5. Minta Hermes memakai tool CRM, misalnya merangkum dealer yang perlu ditindaklanjuti. Tool `read` membaca data; `analyze` menghasilkan usulan. Persetujuan dan pengiriman ke dealer tetap dilakukan manusia di CRM.

Token dapat dicabut kapan saja lewat kartu **Koneksi** di halaman MCP Claude. Jika dicabut, buat token baru dan perbarui konfigurasi di device Hermes.

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
