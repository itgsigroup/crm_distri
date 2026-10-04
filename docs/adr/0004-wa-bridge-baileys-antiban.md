# ADR 0004 — wa-bridge memakai Baileys + kebijakan anti-blokir
Status: diterima (2026-10-04) atas permintaan pemilik. Menggantikan baris "WhatsApp bridge" di ADR 0003 (whatsmeow) dan kembali ke pilihan ADR 0002 (Node + Baileys). Kontrak HTTP bridge ↔ API tidak berubah.

## Konteks
Pemilik meminta WhatsApp memakai **Baileys** dan memastikan risiko nomor **diblokir Meta** dikurangi. Baileys (seperti whatsmeow) memakai protokol linked device yang **tidak resmi**: tidak ada jaminan bebas blokir. WhatsApp menandai akun yang perilakunya mirip pengirim massal: banyak pesan ke orang yang tidak pernah menghubungi, teks identik ke banyak chat, lonjakan volume, aktivitas tengah malam, perangkat baru yang langsung mengirim banyak, sering dilaporkan/diblokir penerima, dan koneksi yang putus-sambung agresif. Risiko diturunkan dengan **berperilaku seperti satu orang sales yang membalas pelanggannya** — bukan dengan menyamar atau mengakali deteksi.

## Keputusan
- `apps/wa-bridge`: Node ≥ 20 + TypeScript, `baileys@7.0.0-rc14` (versi dipin persis), `pg`, `pino`. Endpoint, HMAC, dan format `WaEvent` sama dengan sebelumnya.
- **Auth state di PostgreSQL** (skema `wa_bridge`, tabel `arc_auth`), bukan `useMultiFileAuthState` (yang menurut Baileys tidak untuk produksi). `arc reset` tidak memutus HP.
- **Kirim hanya dengan izin manusia** (tetap): bridge memanggil balik `GET /bridge/actions/{id}`; satu `action_id` hanya pernah terkirim sekali (retry mengembalikan wamid yang sama).

### Penjaga anti-blokir (setiap kirim, setelah approve)
| Aturan | Default | Alasan |
|---|---|---|
| Hanya membalas chat yang pernah menghubungi nomor ini (`first_contact`) | aktif | Pesan pertama ke kontak dingin = pemicu utama laporan spam. Kontak baru dihubungi via email/telepon atau Cloud API (template resmi). |
| Opt-out: "STOP", "berhenti", "jangan hubungi saya lagi" | aktif | Menghormati penerima; pesan masuk berikutnya membuka kembali. |
| Teks identik ke > 3 chat/jam (`broadcast`) | 3 | Pola broadcast. Personalisasi pesan. |
| Jam tenang | 21.00–07.00 WIB | Aktivitas malam tidak wajar untuk sales. |
| Batas per nomor | 20/jam, 120/hari | Volume wajar satu orang. |
| Per chat | 6/jam, jeda ≥ 20 dtk | Tidak membanjiri satu kontak. |
| Pemanasan nomor baru tertaut | 15/hari → 120/hari dalam 7 hari | Perangkat baru yang langsung ramai dicurigai. |
| Jeda acak antar kirim | 2–6 dtk, berurutan per nomor | Tidak ada burst. |
| Presensi "mengetik…" sebelum kirim | 45 ms/karakter (1,5–8 dtk) | Pola manusia. |

Penolakan dijelaskan dalam Bahasa Indonesia ke pengguna (HTTP 403/409/429 + `Retry-After`), dan API tetap memakai batas 20/jam sebagai lapis kedua.

### Kebersihan koneksi
- Nama perangkat stabil (`ARC (Ubuntu)`), `markOnlineOnConnect: false` (HP tetap menerima notifikasi), versi WhatsApp Web terbaru (`fetchLatestBaileysVersion`), riwayat terbaru saja (`syncFullHistory: false`, disaring `history_days`).
- Reconnect dengan backoff eksponensial + jitter (5 dtk → maks 10 mnt). **Tidak reconnect** setelah `loggedOut` (401), `forbidden` (403, kemungkinan dibatasi), `connectionReplaced` (440), `badSession` — status `disconnected` + alasan, watchdog API mengirim email ke CEO setelah 10 menit.
- `cachedGroupMetadata` (TTL 1 jam), daftar grup di-cache 10 mnt, lookup profil ≤ 30/jam/nomor dan di-cache 7 hari — kueri berulang ke server adalah pemicu rate-limit.
- Tidak ada auto-read (centang biru tetap dikendalikan HP), tidak mengunduh media, tidak memposting status, mengabaikan status/broadcast/channel.

### Yang sengaja tidak dilakukan
Rotasi identitas perangkat, proxy/IP berganti, atau teknik lain untuk mengelabui deteksi WhatsApp — itu melanggar ketentuan dan justru memperbesar risiko. Untuk volume keluar yang besar atau kontak baru, jalurnya **WhatsApp Cloud API** (transport B, resmi, template).

## Konsekuensi
- Tambahan runtime Node di samping Go; satu proses kecil, dipantau `/health` (sesi, antrean, pemakaian batas).
- Baileys 7 masih *release candidate*: versi dipin; upgrade mengikuti prosedur di runbook (uji di nomor cadangan dulu).
- Sebagian tindakan yang sudah di-approve bisa ditolak bridge (mis. jam tenang); pengguna melihat alasannya dan dapat mencoba lagi.
- Pemakaian tetap berisiko: gunakan nomor kerja yang sudah lama aktif, jangan nomor baru untuk kirim massal, dan pindahkan nomor bisnis utama ke Cloud API bila volume naik.
