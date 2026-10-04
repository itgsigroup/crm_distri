# Stage 12 — Growth: funnel & flywheel, tender radar, tim, renewal & ekspansi, denyut bisnis

## Tujuan
Melengkapi mesin pertumbuhan: sumber & win rate, verdict flywheel terhitung, tender radar, scorecard tim, sistem terpasang & ruang ekspansi, renewal radar, Denyut bisnis lengkap.

## Baca dulu
`00` (angka acuan), `06` (Research, Forecast), `05-ui-spec` (Prospek & funnel, Pipeline bawah, Relasi ekspansi, Hari ini kanan).

## Kerjakan
1. `Opportunity.source` (tender/inbound/referral/ekspansi/cold) — dari Odoo tag/asal atau ditanya saat create; sumber & win rate 24 bulan; kartu "Funnel atau flywheel" dengan verdict **dihitung** (win rate pelanggan-lama vs lainnya) + 3 metrik (referral rate, expansion rate, time-to-delight).
2. Tender radar v1: impor CSV/URL + kata kunci → match score (Research) → Action `qualify_tender`.
3. Tim & disiplin: per sales — pipeline, respons lead, follow-up tepat waktu, multi-thread, coaching note (LLM heavy, bukti).
4. InstalledSystem (dari SO Won/impor) → garansi habis ≤ 90 hari → Action `offer_maintenance`; matriks ruang ekspansi per akun (heuristik sektor × lini) → Action `expansion_opportunity`; kartu di Relasi & Hari ini (Renewal & ekspansi).
5. Denyut bisnis lengkap (`/metrics/pulse`) dengan delta vs periode sebelumnya.

## Acceptance criteria
- Verdict flywheel berubah bila data dibalik (test). Tender fixture 3 → skor & alasan. Amarta → `offer_maintenance` (garansi Nov). Tim fixture → Fajar "0 lead, 102 pesan".
