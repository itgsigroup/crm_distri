# ADR 0067 — Chat dengan Hermes Agent dari halaman Hermes (seperti Telegram)

Status: diterima · 2026-10-27 · Sam

## Konteks
Sam memakai Hermes Agent lewat bot Telegram. Permintaannya (27 Okt): "halaman hermes agent kita ubah seperti di telegram,
jadi kita chattingan dengan hermes itu". Hermes berjalan di device lain dan membaca GSI Orbit lewat MCP dengan token
manual. Hermes juga punya **API server** bawaan yang kompatibel dengan OpenAI: `POST /v1/chat/completions` di port 8642,
diaktifkan dengan `API_SERVER_ENABLED=true` + `API_SERVER_KEY` lalu dijalankan dengan `hermes gateway`.

## Keputusan
- Halaman Hermes Agent dibagi menjadi tab **Chat** (bawaan) dan **Panduan & koneksi** (isi halaman lama ditambah kartu
  sambungan chat).
- Browser tidak pernah berbicara langsung dengan Hermes. API GSI Orbit menjadi proxy, dan URL serta API key Hermes
  disimpan terenkripsi (`secrets`: `hermes.api_url`, `hermes.api_key`, disegel dengan `SESSION_SECRET`). Hanya CEO yang
  bisa mengisinya, dan isian diperiksa dulu ke Hermes (`GET /v1/models`) sebelum disimpan.
- Riwayat chat disimpan **per pengguna** di tabel `hermes_messages` (migrasi 0067). Chat Completions Hermes bersifat
  stateless, jadi setiap pertanyaan dikirim bersama system prompt singkat dan 30 giliran terakhir yang berhasil (giliran
  demo dan yang gagal tidak ikut). Header `X-Hermes-Session-Key: orbit-user-<id>` memisahkan memori jangka panjang
  Hermes per pengguna.
- Jawaban **asinkron**: `POST /mcp/hermes/chat` menyimpan pertanyaan dan baris balasan `pending`, lalu goroutine
  memanggil Hermes (timeout 10 menit). Halaman melakukan polling tiap 1,5 detik selama ada balasan pending. Alasannya,
  satu run agent dengan beberapa tool MCP bisa lebih lama dari timeout reverse proxy (nginx 60 detik). Balasan yang
  pending lebih dari 10 menit (API dimulai ulang) ditandai gagal. Satu pengguna hanya bisa punya satu pertanyaan yang
  sedang dijawab (409).
- Rute berada di bawah `/api/mcp/hermes/…`, jadi mengikuti hak menu MCP yang sudah ada: `GET/POST/DELETE /mcp/hermes/chat`,
  serta `PUT/DELETE /mcp/hermes/config` (khusus CEO).
- Tanpa Hermes yang tersambung, chat menjawab dalam **mode demo** (`hermes.Fake`) dengan petunjuk cara menyambungkan.
- Jaringan: server GSI Orbit harus bisa menjangkau device Hermes. Yang disarankan adalah Tailscale (gratis, privat,
  `API_SERVER_HOST=0.0.0.0` + IP 100.x). Cloudflare Tunnel bisa dipakai sebagai alternatif.
- Migrasi goose dijalankan dengan `WithAllowOutofOrder(true)`. Alasannya, pekerjaan paralel yang memakai nomor migrasi
  lebih kecil (mis. 0065/0066 yang belum di-merge) tetap bisa diterapkan setelah 0067 sudah jalan di produksi.

## Akibat
- Tim bisa chat dengan Hermes yang sama dengan bot Telegram, langsung di GSI Orbit, dengan riwayat yang hanya terlihat
  oleh dirinya sendiri.
- Hermes tetap hanya membaca dan mengusulkan. Keputusan dan pengiriman ke dealer tetap dilakukan manusia (prinsip §2).
  Token MCP Hermes tidak berubah.
- Pengguna di Orbit dan pemilik device Hermes sama-sama bisa melihat isi percakapan, karena transkrip juga tersimpan di
  sisi Hermes. Isi pertanyaan dikirim apa adanya ke Hermes, termasuk ke provider model yang dipakai Hermes.
- Jawaban belum di-stream per kata. Indikator "sedang mengetik…" muncul sampai jawaban lengkap tiba.
