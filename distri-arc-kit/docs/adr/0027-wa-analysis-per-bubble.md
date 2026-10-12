# ADR 0027 — Analisa WhatsApp per bubble chat

Status: diterima · 2026-10-12

## Konteks
Analisa WhatsApp (ADR 0026) membaca chat per percakapan, hanya di siklus penuh. Di halaman Chat, Sam ingin melihat
analisanya **di setiap bubble**: "tiap kita analisa maka chatnya juga teranalisa". Tombol *Analisa lanjut* untuk
nomor baru mengirim scope `screen:chat`, yang belum dikenal, sehingga gagal.

## Keputusan
- `waanalysis.AnalyzeMessages` (Go murni) membaca setiap pesan: pesan pelanggan → permintaan (ekstraktor AI Order),
  barang & jumlah, dan status balasan: `replied` (berapa menit sampai dibalas), `waiting` / `needs_reply` (menunggu
  sejak pesan pertamanya yang belum dibalas, ambang 2 jam), `closing` ("ok siap 👍", tidak perlu dibalas). Pesan
  sales → `reply` (berapa lama pelanggan menunggu) atau `follow_up` (sales menyapa lebih dulu).
- **Setiap siklus, apa pun scope-nya**, membaca bubble di Ingest: siklus penuh / `screen:*` / `agent:*` semua
  percakapan dealer & nomor baru dalam 7 hari; `dealer:<id>` hanya percakapan dealer itu. Grup tidak pernah dibaca.
  Hanya siklus penuh yang menulis analisa percakapan, total, dan Rencana (ADR 0026 tetap).
- Tabel `wa_message_insights` (migrasi 0021): satu baris per pesan, analisa terakhir menang (upsert, idempoten);
  dihapus bersama chat oleh retensi (tanpa FK karena `chat_messages` dipartisi).
- **AI per bubble**: pesan pelanggan dalam jendela membawa `id` di bacaan Analisa WhatsApp; model di server atau
  Claude lewat MCP (`orchestrator_submit` → `messages: [{id, note, confidence}]`, opsional) menulis catatan ≤ 200
  karakter. Divalidasi terhadap Input, di-unmask, disimpan sebagai AI (ADR 0025: tanpa template).
- Scope `screen:chat` sah (agen AI Order + AI Prospek).
- API `GET /api/chat/threads/{id}`: setiap bubble percakapan 1-on-1 membawa `analysis` (permintaan, barang, catatan
  AI, siklus, waktu analisa); status balasan dibaca **langsung** di halaman terbaru (balasan sesudah analisa langsung
  terhitung), halaman lama memakai status tersimpan. Bubble yang ditulis sesudah analisa terakhir: "belum dianalisa".
- UI: baris analisa di bawah tiap bubble, tombol ✦ *Per pesan* untuk menyembunyikan/menampilkan (diingat di browser).

## Konsekuensi
- Siklus non-penuh kini juga membaca chat 7 hari (murah: Go murni, satu upsert JSON).
- Catatan AI hanya untuk ≤ 20 percakapan paling mendesak per siklus (batas bacaan ADR 0026).
