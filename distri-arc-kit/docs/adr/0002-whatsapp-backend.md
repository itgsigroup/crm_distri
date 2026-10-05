# ADR 0002 — WhatsApp: whatsmeow (linked device) + Cloud API di balik satu interface
**Status**: diterima · 2026-10-05

## Konteks
Order dealer masuk lewat WA nomor sales pribadi/perusahaan, termasuk grup koordinasi gudang. Cloud API resmi tidak mendukung grup dan riwayat; linked-device tidak resmi dan berisiko blokir bila dipakai untuk broadcast.

## Keputusan
`internal/wa.Transport` dengan dua implementasi: **whatsmeow** (Go, multi-device, QR pairing per nomor sales; membaca grup & riwayat) dan **Cloud API** (webhook + Graph API; untuk nomor bisnis resmi). Semua pengiriman hanya dari `outbox` yang disetujui manusia, dengan batas harian dan jeda acak; tidak ada broadcast; tidak ada pembacaan DM internal.

## Konsekuensi
- Pairing ulang bila sesi gugur; health check menampilkan status per nomor.
- Risiko blokir diterima untuk fase pilot; migrasi nomor utama ke Cloud API saat volume naik.
