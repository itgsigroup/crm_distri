# ADR 0074 — Reaksi per pesan seperti WhatsApp

Status: diterima · 2026-10-28 · Sam

## Konteks
Sam: "tolong tambahkan reaction per bubble chat seperti whatsapp". Sebelumnya bridge membuang reaksi (`reactionMessage`),
jadi reaksi pelanggan tidak terlihat di GSI Orbit, dan tim tidak bisa memberi reaksi dari Chat.

## Keputusan
- **Masuk**: bridge memetakan `reactionMessage` menjadi event dengan `react_to` (id pesan yang diberi reaksi) dan `text` =
  emoji (`""` berarti reaksi dibatalkan). Ingest tidak menyimpannya sebagai pesan. Reaksi disimpan di
  `chat_message_reactions` (migrasi 0071), satu emoji per orang per pesan (`reactor` = digit nomor), di setiap salinan
  pesan dengan `wa_msg_id` itu. Bila ada dua reaksi dari orang yang sama, yang terbaru dipakai, karena riwayat bisa datang
  terlambat. Reaksi tidak memicu AI Trigger, Lyra, atau Balas otomatis. Reaksi juga tidak menandai chat belum dibaca dan
  tidak dihitung sebagai pelanggan menulis, jadi opt-out STOP tidak terbuka karena emoji.
- **Keluar**: `PUT /chat/threads/{id}/messages/{mid}/reaction {emoji}` (`""` = batalkan). Reaksi sampai ke pelanggan, jadi
  diperlakukan sebagai keputusan manusia seperti balasan: proposal, outbox `kind = react`, dan audit `chat.react`. Worker
  menjalankannya lewat `wa.Reactor` → bridge `POST /sessions/{id}/react`, dengan kunci pesan yang sama seperti "Balas"
  (`from_me`, dan `participant` untuk grup). Bridge tetap memeriksa persetujuan dan idempotensi per action. Reaksi tidak
  menambah pesan ke chat, jadi tidak dihitung kuota anti-blokir (sama seperti edit dan hapus). Satu emoji per nomor:
  memilih emoji lain menggantikannya, dan memilih emoji yang sama membatalkannya. Tim memberi reaksi atas nama nomor GSI
  chat itu.
- **Tampilan**: tombol 🙂 di samping bubble (muncul saat kursor diarahkan, di sebelah tombol Balas) membuka enam emoji
  WhatsApp (👍 ❤️ 😂 😮 😢 🙏) dan **+** untuk emoji lain. Reaksi tampil sebagai chip di bawah bubble: emoji dan jumlahnya,
  dengan reaksi nomor ini ditandai. Klik chip sendiri untuk membatalkan, atau chip lain untuk ikut. Reaksi langsung tampil
  sebelum WhatsApp mengonfirmasi, dan kembali seperti semula bila gagal.

## Akibat
- Reaksi pelanggan dan rekan terlihat di Chat. Tim bisa membalas dengan reaksi tanpa mengirim pesan baru.
- Setiap reaksi dari Orbit tercatat sebagai proposal yang dieksekusi. Daftar proposal bertambah, sama seperti balasan
  chat biasa.
- Deploy ini memulai ulang wa-bridge.
