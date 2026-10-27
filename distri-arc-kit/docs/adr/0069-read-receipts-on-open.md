# ADR 0069 — Centang biru dikirim saat chat dibuka di GSI Orbit

Status: diterima · 2026-10-27 · Sam · menggantikan satu aturan anti-blokir di ADR 0017 ("dibaca hanya tepat sebelum membalas")

## Konteks
Sejak ADR 0017, tanda terbaca (centang biru) baru dikirim ke WhatsApp tepat sebelum balasan keluar. Balasan itu pun
menunggu antrean penjaga anti-blokir (jeda antar kirim, jam tenang). Akibatnya, customer yang chat dan sudah dibaca
sales di Orbit melihat pesannya "belum dibaca" lama sekali. Sam melaporkannya sebagai bug: tanda terbaca lambat.

## Keputusan
- Saat orang membuka chat di Orbit (`POST /chat/threads/{id}/read`, yang hanya dipanggil oleh pengguna yang login), API
  meminta bridge `POST /sessions/{id}/read {chat_id}`. Bridge mengirim tanda terbaca untuk pesan masuk yang belum
  dibaca di chat itu (maks. 20 terakhir yang ia catat), seperti WhatsApp Web.
- Tetap **tidak** auto-read saat pesan masuk. Tanda terbaca hanya dikirim karena tindakan manusia, yaitu membuka chat. Pesan yang
  masuk saat chat sedang terbuka ikut terbaca karena chat itu memang sedang dilihat.
- DM internal antar karyawan tidak dikirimi tanda terbaca (kebijakan privasi).
- Gagal (bridge putus, nomor belum terhubung) hanya dicatat di log. Chat tetap terbaca di Orbit, dan centang biru tetap
  terkirim saat membalas seperti sebelumnya.

## Konsekuensi
- Customer melihat centang biru segera setelah sales membuka chatnya.
- Risiko blokir tidak bertambah berarti: membaca chat yang dibuka adalah perilaku normal WhatsApp Web, satu permintaan
  per pembukaan chat, dan bukan pesan keluar.
- Setelah bridge restart, pesan yang masuk sebelum restart tidak lagi tercatat sebagai "belum dibaca" di bridge, jadi
  pesan itu baru mendapat centang biru saat dibalas.
