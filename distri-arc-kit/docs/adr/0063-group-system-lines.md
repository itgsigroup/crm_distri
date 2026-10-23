# ADR 0063 — Baris sistem grup: chat grup ada di Orbit sejak grup dibuat

Status: diterima · 2026-10-23 · Sam

## Konteks
Sam (23 Okt): "pada chat tolong perbaiki agar pesan realtime — info terakhir grup yang barusan kita buat belum ada di
system". Jalur realtime (bridge → webhook → `pg_notify('chat_message')` → SSE → refetch) sudah bekerja untuk pesan;
yang belum ada adalah **grupnya**. Bridge hanya meneruskan pesan yang punya isi (`msg.message`), sedangkan grup yang
baru dibuat, diganti nama, atau ditambah anggota hanya mengirim **stub** (`messageStubType` GROUP_CREATE,
GROUP_PARTICIPANT_ADD, …) tanpa isi — dibuang di `toWaEvent`. Akibatnya grup baru tidak punya thread di Orbit sampai
seseorang mengetik pesan teks pertama; "info grup" (nama, deskripsi, anggota) juga tidak pernah tampak.

## Keputusan
- Bridge mengubah stub grup **live** menjadi baris sistem lewat jalur pesan biasa (`apps/wa-bridge/src/events.ts`
  `groupStub`): `wamid` = id stub (idempoten), `media_meta {kind: "system", system: create|subject|icon|description|
  add|join|remove|leave|promote|demote}`, teks seperti WhatsApp Indonesia: "Anda membuat grup "X"", "Rina mengubah
  nama grup menjadi "Y"", "Anda menambahkan Budi, +6285…", "Budi keluar". Nama anggota dari metadata grup (nama di
  HP), "Anda" untuk nomor sendiri, nomor saat tak dikenal. Stub dari history sync **tidak** diteruskan (grup lama
  sudah dikenal; ratusan baris tanpa guna). Stub 1-on-1 dan stub bukan grup (e2e notice, ephemeral) tetap dibuang.
- Server menyimpannya seperti pesan grup lain (klasifikasi internal/eksternal, `wa_groups`, thread per nomor GSI,
  rename grup lewat `GroupName`), tetapi sebagai **milik tak seorang pun**: selalu `direction = 'in'`, `internal =
  false`, unread 0, tanpa signal, tanpa AI Trigger, tanpa deteksi janji, tidak menyentuh kontak. `ListThreads` dan
  Analisa WhatsApp mengecualikan `media->>'kind' = 'system'` dari hitungan "menunggu balasan" — baris sistem tidak
  menunggu jawaban dan tidak menjawab apa pun.
- UI menampilkannya di tengah seperti WhatsApp (`.bub-sys`), tanpa bubble, menu, atau label; daftar chat
  menampilkannya sebagai pesan terakhir tanpa awalan nama (`sender_name` kosong).

## Konsekuensi
- Grup yang dibuat dari HP yang tertaut muncul di Chat beberapa detik setelah dibuat, dengan nama dan anggotanya,
  sebelum ada pesan — lewat jalur realtime yang sama (`chat_message`).
- Ekspor/cadangan chat (ADR 0061) ikut membawa baris ini, seperti ekspor WhatsApp sendiri.
- Grup internal dengan pembacaan dimatikan tetap tidak menyimpan apa pun, termasuk baris sistemnya.
