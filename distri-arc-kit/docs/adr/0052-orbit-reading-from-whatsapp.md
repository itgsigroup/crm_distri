# ADR 0052 — Analisa Orbit dari chat WhatsApp, lewat routine Claude

Status: diterima · 2026-10-19 · Sam

## Konteks
Sam: "pada orbit harusnya analisa dari WhatsApp bukan dari BigQuery, analisanya mengikuti routine Claude, dan pastikan
orbit sesuai MD". Pilihan Sam: posisi dealer tetap dari siklus order (glossary: `cyc` = hari sejak order terakhir ÷
siklus order); yang dari WhatsApp adalah **analisanya**. Halaman Orbit belum punya analisa AI: label "dibaca AI
Follow-up" di "Yang bergerak" hanya perbandingan angka, dan tombol Analisis ulang tidak lewat routine.

## Keputusan
- Bacaan halaman baru **Analisa Orbit** (agent `Analisa Orbit`, screen `orbit`, migrasi 0052 memperluas
  `screen_analyses`). Ikut setiap siklus yang menjalankan AI Follow-up (scope `screen:orbit` dan `all`).
- Input (`orbitReading`): ≤ 20 dealer di orbit yang perlu disapa — Churn / At risk, jadwal order ≤ 7 hari, atau yang
  chat dalam 30 hari (yang chat didahulukan, lalu omzet) — dengan posisinya (status, cyc, siklus order, hari sejak
  order, jadwal) dan **8 pesan WhatsApp terakhirnya** (30 hari, dari `WAWindowMessages`, pelanggan/sales). Nama sales
  dan PII dimasking. Prompt `orbit_read.md`: Claude menjelaskan dari chat kenapa dealer bergeser dan langkah follow-up;
  dilarang mengarang angka/isi chat.
- Ditulis Claude lewat MCP — routine claude.ai (ADR 0049) dipicu saat siklus menunggu; diperiksa terhadap Input,
  disimpan sebagai AI. Halaman Orbit: kartu **Analisa Orbit** (ringkasan) dan catatan per dealer di daftar dealer.
- Tombol "Analisis ulang" di kepala Orbit dihapus: kotak Orchestrator (kanan bawah, scope `screen:orbit`) yang dipakai,
  sama seperti Pusat kendali. Label "Yang bergerak" kini "dihitung dari siklus order · dibanding bulan lalu".
- Sesuai MD (08-frontend): OrbitBoard (lingkar 92/170/248/310, sudut `min(cyc,1)·2π`, titik `7+√sow·1.3`, warna
  `credit_state`), Isi orbit, Yang bergerak — tidak berubah.

## Konsekuensi
- Siklus penuh kini juga meminta Analisa Orbit dari Claude (satu bacaan tambahan per siklus).
- Dealer tanpa chat 30 hari dicatat sebagai "diam"; posisi orbit tetap tidak dipengaruhi chat.
