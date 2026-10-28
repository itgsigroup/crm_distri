# ADR 0073 — Grup "hanya admin yang bisa mengirim pesan"

Status: diterima · 2026-10-28 · Sam

## Konteks
Di grup WhatsApp yang diatur "hanya admin yang dapat mengirim pesan", Orbit tetap membuka kolom ketik bagi nomor GSI yang
bukan admin. Pesan masuk ke Orbit, tetapi WhatsApp menolaknya, sehingga tidak pernah sampai.

## Keputusan
- **Bridge**: `Session.adminOnly(chat)` membaca metadata grup (`announce`) dan memeriksa apakah nomor ini admin. Nomor
  dikenali dari JID telepon atau LID-nya. Metadata di-cache 1 jam dan dibuang saat ada `groups.update` atau
  `group-participants.update`. `Sender.send` menolak pengiriman ke grup seperti itu dengan
  `403 admin_only` sebelum sampai ke WhatsApp. Daftar grup juga membawa `admin_only`.
- **Outbox**: penolakan itu final. Pesan ditandai *gagal* dengan alasan "WhatsApp: Hanya admin yang dapat mengirim
  pesan ke grup ini".
- **API**: detail chat grup memuat `admin_only`, diambil dari cache daftar grup per nomor (ADR 0071). Pengecekan ini
  tidak menunggu WhatsApp: bila cache-nya basi, daftar diperbarui di latar belakang. `POST /chat/threads/{id}/messages`
  ke grup seperti itu dijawab 409, dan bridge tetap menjadi penjaga terakhir.
- **UI**: kolom ketik diganti tulisan "Hanya admin yang dapat mengirim pesan ke grup ini", seperti di WhatsApp.

## Akibat
- Tidak ada lagi pesan yang tampak terkirim di Orbit padahal ditolak WhatsApp. Pesan lama yang sudah tercatat
  terkirim tidak diubah.
- Pengaturan grup yang baru diubah admin langsung terbaca oleh pengecekan kirim di bridge (cache dibuang). Kolom ketik di
  Orbit mengikuti paling lambat 10 menit.
