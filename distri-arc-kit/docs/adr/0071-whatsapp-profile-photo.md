# ADR 0071 — Foto profil WhatsApp di Chat dan profil dengan grup bersama

Status: diterima · 2026-10-28 · Sam

## Konteks
Permintaan Sam (28 Okt): avatar di daftar chat memakai foto profil WhatsApp. Saat foto diklik, tampil profil WhatsApp
seperti di aplikasi WhatsApp, termasuk grup mana saja yang diikuti kontak itu.

## Keputusan
- **Bridge**: endpoint baru `GET /sessions/{id}/photo/{jid}?size=preview|image` memakai `profilePictureUrl` Baileys.
  Gambar diambil dari CDN WhatsApp lalu diteruskan (maks. 2 MB). Jawabannya 404 bila kontak tidak punya foto atau
  menyembunyikannya. Ada batas `BRIDGE_PHOTO_LOOKUPS_PER_HOUR` (bawaan 120) per nomor per jam (anti-blokir, ADR 0004);
  bila terlampaui, jawabannya 429.
- **API** menyimpan foto sebagai cache di tabel `wa_photos` (migrasi 0070, per jid × ukuran). Foto dianggap segar 3 hari,
  status "tidak ada foto" 1 hari. Bila WhatsApp tidak bisa ditanya (nomor putus atau batas tercapai), API memakai foto
  lama bila ada dan tidak menyimpan apa-apa. Maksimal 4 permintaan foto berjalan bersamaan.
  `GET /chat/threads/{id}/photo[?full=1]` mengikuti hak lihat chat (`threadParam`).
- **Profil**: `GET /chat/threads/{id}/profile` berisi nama chat, nomor, nama WhatsApp, Info (about), dan status akun
  bisnis. Data ini dibaca lewat lookup profil bridge yang sudah ada (cache 7 hari, 30/jam). Untuk grup, profil berisi
  jumlah anggota.
- **Grup bersama**: grup dari nomor GSI yang terhubung **dan dilihat pengguna** (`chatViewer.sees`) yang anggotanya
  memuat nomor kontak (daftar grup bridge, cache 10 menit). Setiap grup ditautkan ke chat grupnya di GSI Orbit bila
  pengguna boleh membacanya. Nomor LID tanpa nomor telepon tidak bisa dicocokkan.
- **UI**: `ChatAvatar` menampilkan foto (lazy) atau inisial di daftar chat, header chat, dan panel Lead. Klik avatar
  membuka `ProfileSheet` tanpa membuka chat-nya.

## Akibat
- Saat daftar chat pertama kali dibuka, satu nomor GSI bisa mengirim sampai 120 permintaan foto per jam ke WhatsApp.
  Setelah itu foto dilayani dari cache. Avatar yang belum sempat diambil tetap menampilkan inisial sampai giliran
  berikutnya.
- Deploy perubahan ini me-restart wa-bridge sekali (semua ponsel tersambung ulang).
