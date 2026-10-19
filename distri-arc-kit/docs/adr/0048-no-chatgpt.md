# ADR 0048 — ChatGPT tidak pernah dipakai

Status: diterima · 2026-10-19 · Sam · mencabut ChatGPT dari ADR 0040, 0043 dan 0047

## Konteks
Sam: "jangan pakai ChatGPT". Riwayat produksi menunjukkan ChatGPT API tetap bekerja tanpa diminta: siklus #305
(Analisis ulang dari Pusat kendali, 19 Okt 10.25) diambil alih **ChatGPT API cadangan** setelah Claude lewat MCP tidak
menjawab dalam 10 menit (`mcp.analyst.fallback = openai`), dan "API cadangan · Analisa WhatsApp" berjalan dengan
ChatGPT (00.36, 03.06, 05.06). Ini yang terlihat seperti "analisa Claude MCP tanpa membuka Claude Desktop".

## Keputusan
- `analyst.ChatGPTOff = true`: `LoadOpenAIKey` tidak pernah mengembalikan kunci. Akibatnya opsi kedua ChatGPT (siklus
  dan Analisa WhatsApp) tidak pernah siap, Analisa WhatsApp otomatis dengan ChatGPT gagal alih-alih berjalan, dan
  draft balasan tidak memakai ChatGPT. Satu gerbang di kode, tidak bergantung pada pengaturan di database.
- UI: pilihan AI Claude/ChatGPT (ADR 0047) dihapus — Analisis ulang hanya **Claude lewat MCP**; halaman API & model
  AI tidak lagi menawarkan ChatGPT (opsi kedua: Mati / Claude API; analisa otomatis: Claude).
- `POST /cycles` mengabaikan `ai`; siklus MCP selalu ke claude.ai.
- Test klien ChatGPT tetap ada dan menyalakan gerbang sementara (`chatGPTOn`); `TestChatGPTNeverUsed` menjaga gerbangnya.

## Konsekuensi
- Analisa AI hanya dari Claude: lewat MCP (claude.ai/Desktop, atau routine claude.ai tiap 2 jam yang berjalan di cloud
  Claude tanpa membuka Desktop), atau Claude API bila opsi kedua diset ke Claude API.
- Bila Claude tidak dibuka/tidak menjawab dan opsi kedua Mati, siklus gagal setelah `MCP_WAIT` — tidak ada pengganti.
- Pengaturan produksi `mcp.analyst.fallback = openai` kini tidak berefek (tidak siap); sebaiknya disetel ke Mati.
