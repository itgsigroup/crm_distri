# ADR 0031 — Kirim file, pesan suara, dan polling dari Chat

Status: diterima · 2026-10-13 · melengkapi ADR 0029 (media & edit) dan ADR 0017 (bridge Baileys)

## Konteks
Permintaan: dari Chat bisa mengirim file, pesan suara (VN), polling (vote), dll. seperti WhatsApp. Sebelumnya Chat
hanya mengirim teks.

## Keputusan
- **Satu jalur kirim**: `POST /chat/threads/{id}/messages` menerima JSON `{body, reply_to, poll}` atau multipart
  `{file, body (keterangan), reply_to, voice=1, seconds}`. Tetap proposal `reply` yang disetujui pengirim, outbox, dan
  pemeriksaan persetujuan + penjaga anti-blokir di bridge (polling/file dibandingkan sebagai "teks sama" lewat
  pertanyaan atau nama file).
- **File sementara**: isi file disimpan di `chat_outgoing_files` (bytea, maks. 25 MB) hanya sampai outbox
  mengirimnya, lalu dihapus; pesan terkirim menyimpan referensi unduh WhatsApp seperti media yang masuk (ADR 0029).
  Sisa file yang gagal/dibatalkan dihapus job retensi setelah 7 hari.
- **Jenis**: jpeg/png/webp → foto; mp4/3gpp/mov → video; audio → audio; lainnya → dokumen (nama file dipertahankan).
- **Pesan suara**: direkam di browser (MediaRecorder: ogg/opus di Firefox, webm/opus di Chrome, mp4 di Safari);
  bridge mengubahnya ke ogg/opus dengan `ffmpeg` (satu-satunya format VN yang diputar HP), presence "merekam…".
- **Polling**: pertanyaan + 2–12 pilihan berbeda, satu atau beberapa jawaban (Baileys `poll`). Polling yang masuk dari
  pelanggan/grup tampil sebagai pertanyaan dan pilihan.
- **nginx**: `client_max_body_size 30M`, `Permissions-Policy microphone=(self)` (sebelumnya mikrofon diblokir), CSP
  `media-src 'self' blob:` (video/VN di bubble diputar dari blob).

## Konsekuensi
- Hasil vote polling belum dihitung di GSI Orbit (suara polling WhatsApp terenkripsi per pemilih dan perlu
  `messageSecret` polling; dilihat di HP). Lihat OPEN-QUESTIONS.
- Kirim stiker, lokasi, dan kontak belum dibuat.
- File > 25 MB dikirim lewat HP.
