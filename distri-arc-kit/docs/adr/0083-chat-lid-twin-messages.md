# ADR 0083 — Pesan lama dari chat kembar dimuat dari GSI Orbit

Status: diterima · 2026-10-30 · Sam · melengkapi ADR 0060 dan 0072

## Konteks
Di chat "Sukma Abadi" milik Nurul Aini, header menulis "Riwayat sejak 30 Okt" (hari ini) dan hanya ada tombol
"Muat pesan lama dari HP". Pesan lama orang itu tersimpan di GSI Orbit, tetapi di chat lain pada nomor yang sama:
pasangan chat LID (`…@lid`) dan chat nomor yang belum digabung, karena perangkat belum tahu LID itu milik nomor mana
(ADR 0072). Sam meminta pesan lama dimuat dari sistem Orbit dulu.

## Keputusan
- Saat chat 1:1 dibuka, Orbit mencari **chat kembar** di nomor GSI yang sama: pasangan LID ↔ nomor dengan nama
  simpanan atau nama profil yang sama (bukan "Nomor …", harus mengandung huruf). Kembar dihitung hanya bila
  **tepat satu**. Bila ada dua atau lebih chat dengan nama itu, tidak ada yang ditautkan karena bisa jadi orangnya
  berbeda.
- Pesan chat kembar ikut tampil di chat ini (`ListThreadMessages.twins`). Akibatnya `has_more` benar, dan pesan lama
  dimuat dari Orbit lewat tombol **Muat pesan lama** atau dengan menggulir ke atas. Tombol ini juga berfungsi bila chat
  terlalu pendek untuk digulir.
- Penautan ini **hanya untuk tampilan**: tidak ada data yang dipindah. Penggabungan permanen tetap menunggu WhatsApp
  memetakan LID ke nomor (ADR 0072). Membuka chat juga memicu pertanyaan itu di latar belakang, paling sering sekali
  per 30 menit.
- **Muat pesan lama dari HP** baru muncul di awal riwayat Orbit. Titik acuannya adalah pesan tertua di chat ini atau
  di chat kembarnya, dengan JID chat tempat pesan itu disimpan.

## Akibat
- Balasan tetap dikirim ke JID chat yang dibuka.
- Pencocokan nama bisa keliru bila dua orang berbeda memakai nama sama di nomor yang sama dan salah satunya masih LID.
  Risiko ini dibatasi oleh syarat "tepat satu", dan tidak ada data yang diubah.
