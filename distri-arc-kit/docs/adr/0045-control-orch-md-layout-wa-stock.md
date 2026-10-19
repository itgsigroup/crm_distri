# ADR 0045 — Pusat kendali dan Orchestrator persis 08-frontend.md: analisa dari WhatsApp, stok dari BigQuery

Status: diterima · 2026-10-19 · Sam · mengubah pembaruan ADR 0026 (12 Okt), sebagian ADR 0032, dan letak Orchestrator di Pusat kendali (16 Okt)

## Konteks
Sam meminta Pusat kendali dan Orchestrator kembali **persis** seperti `docs/design/08-frontend.md` / `00-overview.md`
(Rencana hari ini sempat hilang), dengan analisa dari WhatsApp. BigQuery hanya dipakai untuk data stok, jadi analisa
stok berasal dari BigQuery.

## Keputusan
- **Pusat kendali** memakai komponen dan urutan MD: kartu Orchestrator (di hero, bukan dock), Rencana hari ini,
  Keputusan, Ringkasan, Jadwal order · 7 hari, Lewat jadwal, KPI utama, Agenda sales, Push stok, Limit tipis.
  Dock tidak tampil di `/` (sesuai MD).
- Sumber tiap kartu:
  | Kartu | Sumber |
  |---|---|
  | Rencana hari ini | `plan_items` yang usulannya tertaut ke percakapan WhatsApp (atau langkah chat) |
  | Keputusan | usulan Claude (MCP) yang tertaut ke percakapan WhatsApp |
  | Ringkasan | ringkasan WhatsApp tulisan Claude (MCP) |
  | Jadwal order · 7 hari | chat yang meminta order (7 hari jendela Analisa WhatsApp) |
  | Lewat jadwal | chat yang menunggu lebih dari batas balas |
  | KPI utama | KPI WhatsApp (terbalas, waktu balas, menunggu) |
  | Agenda sales | chat menunggu per sales |
  | **Push stok** | stok BigQuery (`/stock/push`) — satu-satunya kartu dari BigQuery |
  | Limit tipis | chat yang meminta rilis kredit atau retur |
- Label permintaan per chat (order / rilis kredit / retur) kembali ditampilkan; ini membatalkan larangan ADR 0032 atas
  kartu permintaan untuk dua halaman ini. Teks analisa tetap hanya dari Claude lewat MCP.
- **Orchestrator** memakai komponen MD: header, Pipeline, Resolusi konflik, Riwayat analisis (siklus), kartu Agen
  dengan *Jalankan ulang*, panel MCP, Matriks otonomi. Kartu Analisa WhatsApp (ringkasan, daftar percakapan, riwayat,
  grid sales) dilepas dari halaman ini — ringkasannya ada di Pusat kendali, percakapannya di Chat.
- Halaman lain (Orbit, Dealer, Kredit, Dashboard) tidak berubah.

## Konsekuensi
- Pengguna tanpa akses Chat melihat kartu Pusat kendali berisi pesan akses, kecuali Push stok.
- Label permintaan dibaca dari kata kunci pesan (`agents.Extract`), bukan AI; kartu-kartu itu menampilkan daftar chat,
  bukan kesimpulan.
