# ADR 0072 — Chat ganda karena LID digabung ke chat nomor

Status: diterima · 2026-10-28 · Sam

## Konteks
Sam mencari "rizal" dan menemukan dua chat "Mas Rizal IT - GSI" di nomor yang sama. Satu chat memakai nomor
(+62 823…0530), satu lagi berlabel "Nomor baru". WhatsApp kadang mengirim pesan 1:1 dengan **LID** (`…@lid`) sebelum
perangkat tertaut mengetahui nomor kontaknya. Bridge hanya menerjemahkan LID bila pemetaannya sudah ada. Bila belum, chat
disimpan dengan `wa_jid` LID, terpisah dari chat nomor yang dibuat belakangan.

## Keputusan
- Bridge mendapat endpoint `POST /sessions/{id}/lids {lids}` → `{map: {lid: phone jid}}` yang membaca pemetaan LID lokal
  perangkat (`lidMapping.getPNForLID`). Endpoint ini tidak mengirim query ke WhatsApp, jadi tidak menambah risiko blokir.
- Setiap kali daftar chat dimuat (`GET /chat/threads`), chat 1:1 ber-`wa_jid` LID ditanyakan ke perangkat nomor GSI-nya
  di latar belakang. Tiap chat ditanya paling sering sekali per 30 menit.
- `wa.MergeLIDThread` berjalan dalam satu transaksi.
  - Bila nomor sudah punya chat, isi chat LID dipindahkan ke sana: pesan, bintang, analisis per pesan, insight, bacaan
    Lyra, label chat (label yang ada di kedua chat disimpan sekali), label pesan, pin pesan, dan pin chat. Chat nomor juga
    mengambil yang belum dimilikinya: jenis chat, dealer, kontak, sales, nama simpanan, judul, identifikasi, waktu
    terakhir, jumlah belum dibaca, mention, dan tanda dibalas. Setelah itu chat LID dihapus.
  - Bila nomor belum punya chat, chat LID cukup berganti `wa_jid` ke nomor itu.
  - Menjalankan ulang tidak mengubah apa-apa (idempoten).
- Setelah digabung, server mengirim event `chat_message` sehingga daftar chat langsung segar.

## Perubahan 28 Okt (siang) — LID yang belum diketahui perangkat
Setelah deploy pertama, chat Sam masih dua karena perangkat Granike belum punya pemetaan untuk LID itu. Di Baileys,
LID→nomor hanya bisa dibaca dari data lokal, sedangkan nomor→LID bisa ditanyakan ke WhatsApp lewat usync (cara yang
sama dengan sinkron kontak). Karena itu, Orbit sekarang juga mengirim **nomor kandidat**: chat bernomor di nomor GSI
yang sama yang namanya sama dengan nama simpanan atau nama profil chat LID (maks. 50). Bridge menanyakan LID kandidat
itu (`getLIDsForPNs`, satu query untuk yang belum diketahui). Chat digabung hanya bila WhatsApp menyebut LID itu milik
nomor tersebut. Nama hanya dipakai untuk memilih siapa yang ditanyakan.

## Akibat
- Duplikat yang sudah ada di produksi tergabung begitu daftar chat nomor itu dibuka. LID yang belum diketahui perangkat
  tetap terpisah sampai pemetaannya masuk.
- Penggabungan memindahkan data di database GSI Orbit dan tidak bisa dibatalkan otomatis. Tidak ada data yang dibuang,
  kecuali label atau pin chat yang dobel.
- Deploy ini me-restart wa-bridge.
