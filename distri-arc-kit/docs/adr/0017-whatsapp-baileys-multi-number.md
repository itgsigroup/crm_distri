# ADR 0017 — WhatsApp lewat bridge Baileys, banyak nomor, penjaga anti-blokir
**Status**: diterima · 2026-10-06 · atas permintaan pemilik. Menggantikan pilihan transport whatsmeow di ADR 0002
sebagai transport utama (whatsmeow dan Cloud API tetap tersedia lewat `WA_TRANSPORT`). ADR 0007 (kirim hanya lewat
outbox yang disetujui) tetap berlaku dan diperkuat.

## Konteks
Pemilik meminta Chat memakai **Baileys** dengan **banyak nomor** dan risiko diblokir Meta seminimal mungkin. Baileys,
seperti whatsmeow, memakai protokol perangkat tertaut yang **tidak resmi** — tidak ada jaminan bebas blokir. WhatsApp
menandai akun yang perilakunya seperti pengirim massal: pesan ke orang yang tidak pernah menghubungi, teks identik ke
banyak chat, lonjakan volume, aktivitas malam, perangkat baru yang langsung ramai, sering dilaporkan, dan koneksi yang
putus-sambung agresif. Risiko diturunkan dengan **berperilaku seperti satu orang yang membalas pelanggannya**, bukan
dengan mengakali deteksi. Bridge Baileys dari ARC v1 (`apps/wa-bridge`, sudah teruji) dipakai ulang.

## Keputusan
1. **`apps/wa-bridge`** (Node ≥ 20, TypeScript, `baileys@7.0.0-rc14` dipin persis, `pg`, `pino`): satu sesi per nomor,
   auth state di PostgreSQL (skema `wa_bridge`, bukan file), nama perangkat "Distri ARC", `markOnlineOnConnect: false`,
   riwayat terbaru saja, cache grup & profil, reconnect backoff eksponensial, **tidak** reconnect setelah logout /
   403 / diganti perangkat lain. Layanan systemd sendiri (`127.0.0.1:8111`).
2. **Transport `baileys` di worker** (`internal/wa/baileys.go`): menerima event bridge di `127.0.0.1:8112`
   (`POST /webhooks/wa`, HMAC-SHA256 `BRIDGE_SECRET`), dan **menjawab pemeriksaan sebelum kirim**
   (`GET /bridge/actions/{outbox id}`): hanya baris outbox WhatsApp milik proposal yang diputuskan (approved / edited /
   executed) dan belum terkirim. Setiap kirim membawa id outbox (`wa.WithAction`); tanpa itu transport menolak.
3. **Penjaga anti-blokir** (bridge, setelah persetujuan manusia): hanya membalas kontak yang pernah menghubungi nomor
   itu; opt-out "STOP/berhenti"; teks identik ke > 3 chat/jam ditolak; jam tenang 21–07 WIB (plus jendela kirim 08–18
   Distri ARC); 20/jam & 120/hari per nomor; 6/jam & jeda ≥ 20 dtk per chat; pemanasan nomor baru 15/hari naik 7 hari;
   jeda acak 2–6 dtk berurutan per nomor; presensi "mengetik…" sebanding panjang teks; tidak auto-read, tidak unduh
   media. Penolakan jeda (dengan `Retry-After`) menunda job outbox; penolakan final (kontak dingin, opt-out, broadcast)
   menggagalkan baris dengan alasan dan toast — tidak dicoba ulang.
4. **Banyak nomor**: percakapan milik nomor (`chat_threads.account`, unik per nomor + chat), bukan hanya sales — satu
   sales boleh beberapa nomor, nomor tim (CS kantor) tanpa sales. Pengaturan → WhatsApp: tambah / pasangkan (QR) / lepas
   nomor; penghitung anti-blokir per nomor. Chat: bar nomor (filter, pasangkan langsung dengan QR), "via nomor" per
   percakapan. Sales hanya melihat dan memasangkan nomornya sendiri.
5. Status antre yang lebih tua dari status terkirim dibuang (tidak ada "logout" basi setelah pairing ulang); saat worker
   menyala, nomor tanpa sesi di bridge ditandai `unpaired` (tidak ada "terhubung" palsu).

## Yang sengaja tidak dilakukan
Rotasi identitas perangkat, proxy/IP berganti, kirim ke kontak dingin, broadcast, atau teknik lain untuk mengelabui
deteksi WhatsApp — melanggar ketentuan dan justru memperbesar risiko. Volume besar atau kontak baru: WhatsApp Cloud API
resmi (`WA_TRANSPORT=cloudapi`, template).

## Konsekuensi
- Runtime Node tambahan di samping Go (satu proses kecil); `make check` menjalankan test bridge.
- Baileys 7 masih release candidate: versi dipin; upgrade diuji dulu di nomor cadangan.
- Sebagian pesan yang sudah disetujui bisa ditolak penjaga; pengguna melihat alasannya.
- Pakai nomor kerja yang sudah lama aktif, jangan nomor baru untuk volume tinggi.
