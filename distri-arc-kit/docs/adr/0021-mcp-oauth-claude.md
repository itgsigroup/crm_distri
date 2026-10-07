# ADR 0021 — MCP untuk Claude: OAuth 2.1, tool yang ramah analisis, halaman MCP Claude

Status: diterima · 2026-10-07 · melengkapi ADR 0004 dan 0009 (token bearer tetap ada)

## Konteks
GSI ingin Claude menganalisis semua data Distri ARC lewat MCP. Server MCP (Stage 07) hanya menerima token bearer statis:
claude.ai dan Claude Desktop (custom connector) memerlukan alur OAuth, sehingga hanya Claude Code dengan header manual
yang bisa terhubung. Selain itu nama tool memakai titik (`dealer.list`, ditolak Claude yang menerima `[A-Za-z0-9_-]`),
beberapa tool mengembalikan ribuan dealer sekaligus, dan di belakang nginx pengaman DNS-rebinding SDK menolak request
(API mendengar di loopback, Host = domain publik).

## Keputusan
1. **Authorization server OAuth 2.1 sendiri** (`internal/oauth`, stdlib): metadata resource (RFC 9728) di
   `/.well-known/oauth-protected-resource[/mcp]`, metadata server (RFC 8414) di `/.well-known/oauth-authorization-server`,
   pendaftaran klien dinamis (RFC 7591) `/oauth/register` — klien publik saja, redirect https atau loopback, 100/jam —
   `/oauth/authorize` (PKCE S256 wajib, redirect_uri harus persis terdaftar) dan `/oauth/token` (authorization_code,
   refresh_token). 401 dari `/mcp` membawa `WWW-Authenticate: Bearer resource_metadata=…`.
2. **Persetujuan manusia** di `/claude/izin` (aplikasi web, butuh login): hanya pengguna dengan menu **MCP Claude** dan
   cakupan **semua data** (Claude membaca semua dealer); scope `orchestrate` hanya untuk pemegang hak kebijakan.
   Kode sekali pakai 5 menit (disimpan hash), permintaan 10 menit.
3. **Satu koneksi = satu baris `mcp_clients` (kind `oauth`)**: token akses 1 jam (format `arc_…`, argon2id), refresh
   token 90 hari yang berputar setiap dipakai (sha256), tercatat di `mcp_calls`/audit seperti token manual, dicabut
   dari halaman MCP Claude (refresh ikut mati). Pengguna dinonaktifkan → refresh ditolak.
4. **Tool**: nama didaftarkan dengan `_` (`dealer.list` → `dealer_list`); daftar dihalaman (`limit` 50/maks 500,
   `offset`, `total`, `next_offset`); tool baru `data_ringkasan` (semua data teragregasi), `penjualan_bulanan`,
   `produk_terlaris`, `piutang_ringkas`. Tidak ada tool yang memutuskan atau mengirim (tetap ADR 0004).
5. `DisableLocalhostProtection` di handler MCP: semua request MCP diautentikasi token; perlindungan itu untuk server lokal.
6. **Halaman MCP Claude** (`/claude`, menu `mcp` di master peran): status, langkah claude.ai/Desktop, Claude Code, token
   manual, koneksi (cabut), izin & batas (`mcp.permissions`), daftar tool, contoh prompt, log panggilan.

## Konsekuensi
- nginx/Caddy meneruskan `/.well-known/oauth-*` dan `/oauth/` ke API (bukan 404 lagi). OpenID tidak ditawarkan.
- Klien lama yang memanggil nama bertitik harus memakai nama baru.
- Tanpa `PUBLIC_URL`, header `resource_metadata` tidak dikirim (dev); produksi wajib `PUBLIC_URL` (sudah).
