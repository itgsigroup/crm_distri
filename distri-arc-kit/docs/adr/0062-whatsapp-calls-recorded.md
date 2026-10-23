# ADR 0062 — Panggilan WhatsApp dicatat di chat; tombol Telepon lewat perangkat pengguna

Status: diterima · 2026-10-23 · Sam

## Konteks
Sam bertanya apakah GSI Orbit bisa telepon. Nomor GSI tersambung lewat Baileys (perangkat tertaut, ADR 0004).
Baileys hanya menerima **sinyal** panggilan (offer, accept, reject, timeout, terminate), bisa menolak panggilan dan
membuat link panggilan, tetapi **tidak membawa suara** (tidak ada WebRTC/audio). Menelepon dari browser hanya bisa
lewat layanan berbayar: WhatsApp Cloud API Calling (nomor bisnis resmi, per menit untuk panggilan keluar, nomor itu
tidak lagi dipakai di aplikasi HP) atau penyedia VoIP/SIP — keputusan Sam, bukan dibuat di sini.

## Keputusan
- **Riwayat panggilan** (gratis, sekarang): bridge mengikuti tiap panggilan 1-on-1 dari `offer` sampai selesai
  (`apps/wa-bridge/src/calls.ts`) dan mengirim **satu** event saat selesai: `wamid = call:<id>` (idempoten), media
  `{kind: "call", call: missed|answered|rejected, seconds, video}`, teks seperti WhatsApp ("Panggilan suara tak
  terjawab", "Panggilan suara masuk · 3:12", "Panggilan video ditolak"). Diangkat = `accept` lalu `terminate`;
  `terminate` tanpa `accept` setelah > 60 detik (dering habis = `timeout`) dianggap diangkat di HP, durasi tak
  diketahui. Panggilan grup, panggilan yang dibuat nomor GSI sendiri, dan nomor yang tak bisa di-resolve (LID) tidak
  dicatat.
- Bridge **tidak pernah** mengangkat atau menolak panggilan — HP yang berdering dan bicara.
- Di server panggilan masuk lewat jalur pesan biasa (privasi, nomor internal, idempoten sama). Tak terjawab/ditolak =
  pesan masuk: chat naik, unread, masuk **Belum dibalas**. Diangkat = `replied_mark` sampai akhir panggilan (seperti
  "sudah dibalas"). Panggilan tidak memicu AI Trigger maupun deteksi janji; tidak bisa dibalas-kutip, diteruskan,
  atau dijadikan titik muat riwayat (`call:` dikecualikan).
- Opt-out (STOP) pelanggan tidak berubah oleh panggilan; chat ditandai "dikenal" untuk penjaga anti-blokir.
- **Tombol Telepon** di header chat 1-on-1: `tel:+62…` — menelepon dari perangkat pengguna (HP, atau laptop dengan
  aplikasi telepon). Tidak ada yang dikirim oleh GSI Orbit.

## Konsekuensi
- Perlu dibuktikan dengan HP sungguhan bahwa `accept` sampai ke perangkat tertaut saat HP mengangkat (OPEN-QUESTIONS).
- Panggilan keluar dari HP belum tercatat. Telepon dari browser butuh keputusan layanan berbayar (OPEN-QUESTIONS).
