# Menghubungkan ChatGPT ke ARC

Dua cara: **Custom GPT** (Actions + OpenAPI) atau **connector MCP** (bila workspace ChatGPT Anda mendukung connector kustom). Keduanya hanya bisa membaca dan mengusulkan — keputusan tetap di web ARC oleh manusia.

## A. Custom GPT dengan Actions (OpenAPI 3.1)
1. Web ARC → **Pengaturan → MCP & API** → **Buat API key**, scope `read` (+ `propose` bila GPT boleh mengusulkan tindakan). Simpan key `arc_live_…`.
2. ChatGPT → Explore GPTs → **Create** → Configure → **Actions → Create new action**.
3. **Import from URL**: `https://<domain-arc>/openapi.json`.
4. **Authentication**: API Key → Auth type **Bearer** → tempel key.
5. Instruksi GPT yang disarankan:
   > Kamu asisten penjualan GSI. Jawab dalam Bahasa Indonesia. Untuk pertanyaan tentang akun, deal, kas, atau risiko, panggil `ask` atau endpoint terkait dan selalu sebutkan bukti (evidence) yang dikembalikan ARC. Kamu tidak bisa menyetujui atau mengirim apa pun; arahkan pengguna ke web ARC untuk keputusan.
6. Endpoint yang tersedia: `POST /ask`, `GET /accounts/{id}/brief`, `GET /signals`, `GET /actions`, `POST /actions` (usulan, scope `propose`), `POST /events`. `POST /actions/{id}/decision` butuh scope `human` yang **tidak pernah** diberikan ke mesin → 403.

## B. Connector MCP (OAuth)
Bila tersedia di workspace Anda: Settings → Connectors → **Create** → URL `https://<domain-arc>/mcp`, autentikasi **OAuth**. Login ARC akan diminta; token mengikuti peran & cabang Anda. Daftar tools sama dengan `docs/connect-claude.md`.

## Batasan & keamanan
- Rate limit per key; setiap panggilan tercatat di audit log dan "terakhir dipakai" di Pengaturan.
- Cabut key kapan saja di Pengaturan → MCP & API.
- Webhook keluar (`action.proposed`, `commitment.overdue`, `signal.detected`) ditandatangani HMAC-SHA256 di header `X-ARC-Signature`.
