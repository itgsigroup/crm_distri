# ADR 0081 — Chat: aturan 24 jam WhatsApp, Orbit tidak memulai chat

Status: diterima · 2026-10-30 · Sam ("tolong batasi memulai chat 24 jam seperti peraturan WhatsApp agar nomor tidak di banned").

## Konteks
WhatsApp membatasi nomor bisnis: pesan ke seseorang hanya boleh dalam 24 jam setelah orang itu terakhir menulis
(customer service window). Memulai atau menghidupkan lagi chat dari perangkat tertaut — pesan ke kontak yang belum
pernah menulis atau sudah lama diam — adalah pola yang paling sering membuat nomor diblokir.

## Keputusan
- Chat satu-satu dengan pelanggan (`kind` `dealer` atau `new`) hanya bisa dikirimi dari Chat GSI Orbit bila pesan
  masuk terakhir (`chat_messages.direction = 'in'`) di percakapan itu kurang dari 24 jam (`api.replyWindow`).
- Di luar jendela, `POST /chat/threads/{id}/messages` (teks, file, polling, kontak, teruskan) ditolak `409
  reply_window_closed` sebelum menyentuh outbox. Detail chat membawa `reply_closed` dan `last_in_at`; kotak ketik diganti
  banner kuning dengan tombol **Buka WhatsApp Web** (`web.whatsapp.com/send?phone=…`) dan saran memulai dari HP.
- Begitu pelanggan membalas, jendela terbuka lagi otomatis.
- Grup dan chat internal tidak terikat aturan ini.

## Akibat
- Sales yang ingin menyapa dealer yang sudah lama diam harus memulai dari HP atau WhatsApp Web; balasan dealer membuka
  chat di Orbit.
- Belum dibatasi: pengiriman lewat usulan Orchestrator (follow-up) dan jalur lain di luar layar Chat — tetap lewat
  keputusan manusia dan penjaga anti-blokir yang ada.
