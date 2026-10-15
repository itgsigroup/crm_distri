# ADR 0029 — Media dan edit pesan di halaman Chat

Status: diterima · 2026-10-13 · melengkapi ADR 0017 (bridge Baileys) dan ADR 0028 (riwayat 1 tahun)

## Konteks
Permintaan: di bubble chat bisa **melihat gambar, video, dan pesan suara (VN)** dari pelanggan, dan **mengedit pesan**
seperti di WhatsApp; di daftar chat ada filter **Belum dibalas**. Sebelumnya bridge hanya meneruskan jenis media
("[gambar]", "[pesan suara]") tanpa cara mengambil isinya, dan pesan yang diedit (oleh pelanggan atau dari HP) diabaikan.

## Keputusan
- **Isi media tidak pernah disimpan.** Bridge menyimpan bersama pesan sebuah *referensi unduh* (`media.ref`): protobuf
  `WebMessageInfo` berisi kunci pesan dan field CDN WhatsApp (`directPath`, `url`, `mediaKey`, hash, mime) — tanpa
  caption, thumbnail, atau konteks. Disimpan di `chat_messages.media` (jsonb, sudah ada sejak 0001), ikut terhapus oleh
  retensi chat (365 hari) dan penghapusan PDP.
- **Diambil saat dibuka.** `GET /chat/threads/{id}/messages/{mid}/media` (akses sama dengan percakapannya) meminta bridge
  `POST /sessions/{id}/media` lewat nomor yang menerima pesan itu. Bridge mengunduh dan mendekripsi dari CDN WhatsApp
  (`downloadMediaMessage`, file kedaluwarsa diminta ulang ke HP lewat `updateMediaMessage`) dan meneruskannya sebagai
  stream; API meneruskan ke browser dengan `Cache-Control: private`. Maksimal 4 unduhan bersamaan per nomor (seperti
  WhatsApp Web). Foto dimuat saat terlihat (`loading=lazy`); video, VN, dan dokumen hanya saat diklik.
- **Keamanan tampilan**: hanya tipe gambar/video/audio/PDF umum yang tampil inline; tipe lain selalu diunduh sebagai
  file (`application/octet-stream`, `attachment`), plus `nosniff` dan CSP `sandbox`. `ref` tidak pernah dikirim ke
  browser. LLM tetap hanya membaca teks (label + caption).
- **Edit dari Chat = keputusan manusia baru.** Pesan teks dari nomor itu bisa diedit: yang masih di antrean outbox
  diubah teksnya sebelum terkirim; yang sudah terkirim (≤ 15 menit, batas WhatsApp) menjadi proposal `reply` yang
  disetujui pengedit (`decided_by`, audit `chat.edit`) + baris outbox `kind=edit`. Worker mengirim lewat
  `POST /sessions/{id}/edit`; bridge memeriksa persetujuan ke API seperti kiriman biasa, idempoten per action. Tanpa
  batas laju (tidak menambah pesan di chat) tetapi antre di belakang kiriman nomor itu. Edit yang terlambat dijalankan
  (> 15 menit) ditolak worker.
- **Edit dari WhatsApp ikut tercatat.** Bridge memetakan `protocolMessage` `MESSAGE_EDIT` menjadi event `edit_of`;
  ingest mengganti teks pesan asli dan mengisi `chat_messages.edited_at` (migrasi 0025). Bubble menampilkan "diedit".
- **Belum dibalas** = percakapan satu-satu (dealer, nomor baru) yang punya pesan pelanggan setelah balasan terakhir kita
  (`waiting_since`). Grup tidak dihitung: obrolan grup bukan permintaan ke satu nomor.

- **Balas (reply)** — tambahan 13 Okt: `chat_messages.quoted_wa_id` (migrasi 0027) menyimpan id WhatsApp pesan yang
  dikutip (bridge membaca `contextInfo.stanzaId` dari semua jenis pesan). Balasan dari Chat membawa `quote {id, from_me,
  participant, text}` di payload outbox; bridge mengirimnya dengan opsi `quoted` Baileys. Pesan yang belum punya id
  WhatsApp (masih antre) tidak bisa dikutip.

- **Hapus untuk semua orang** — tambahan 13 Okt: `chat_messages.deleted_at` (migrasi 0028). Dari Chat: pesan dari
  nomor itu, ≤ 48 jam (WhatsApp ±2 hari) — keputusan manusia lewat outbox `kind=delete`, bridge mengirim `{delete: key}`;
  balasan yang masih antre dibatalkan (`outbox.status = cancelled`). Dari WhatsApp: `protocolMessage` `REVOKE` →
  `revoke_of`. Teks asli tidak dihapus dari database (audit), hanya tidak ditampilkan.

## Konsekuensi
- Media dari pesan yang tersimpan sebelum perubahan ini belum punya `ref` ("buka di ponsel"); referensinya terisi
  saat WhatsApp mengirim pesan itu lagi (sinkron riwayat / `wa.backfill`).
- Transport Cloud API dan fake belum bisa mengambil media (API menjawab 409). Lihat OPEN-QUESTIONS.
- Media hanya bisa dibuka selama nomor penerima terhubung ke bridge.

## Revisi 2026-10-15 — file yang dikirim dari Chat disimpan 30 hari
- Bug: gambar yang dikirim dari Chat tidak bisa dilihat pengirimnya — selama di antrean kirim bubble hanya "mengirim…",
  dan setelah terkirim file salinan langsung dihapus sehingga tampilan bergantung pada referensi WhatsApp.
- File yang dikirim dari Chat (`chat_outgoing_files`) kini **tetap disimpan 30 hari** (`ops.OutgoingKeepDays`) dan
  ditampilkan apa adanya — sejak masuk antrean sampai sesudah terkirim. Setelah 30 hari (job retensi harian) bubble
  kembali diambil dari WhatsApp lewat referensinya. Media yang **diterima** tetap tidak disimpan (keputusan awal ADR ini).
