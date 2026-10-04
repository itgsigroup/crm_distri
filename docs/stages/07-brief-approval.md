# Stage 07 — Brief harian, approval lengkap, kalibrasi, notifier

## Tujuan
Hari ini menjadi lengkap: brief 4 poin (06.45 & 16.00) dikirim ke email/Basecamp, Keputusan sebagai pusat approval (termasuk credit guardrail), Forecast, Besok, Denyut bisnis v1.

## Baca dulu
`01` (kadens, model tindakan), `06` (Chief/Brief), `04` (human-only), `05-ui-spec` (Hari ini).

## Kerjakan
1. Brief agent: 4 poin (gap target, risiko, orang/champion, peluang) dengan evidence; ringkasan sumber; confidence; render HTML email + teks Basecamp/WA-friendly. `arc brief --send=false`.
2. `Notifier`: SMTP akun sistem ARC, Basecamp (opsional), Fake; penerima per role.
3. Credit guardrail: Action `credit_release` dari event distribusi (manual/`POST /events`) dengan exposure/limit/pola bayar (dari data ARC; Odoo di tahap 08) & checklist SOP-SEC-001; keputusan `approve_with_dp|hold|reject`.
4. Hari ini lengkap: status strip, tur, Brief, Keputusan dengan filter, Komitmen, Sinyal, Besok (dari kalender manual + Meeting prep), Forecast, Denyut bisnis v1 (respons lead, akurasi commit, waktu identifikasi), Renewal & ekspansi placeholder (tahap 12), baris agen.
5. Kalibrasi lengkap di Pengaturan → MCP & API: acceptance rate per agen + "Yang dipelajari".

## Acceptance criteria
- `arc brief` dari seed → 4 poin, tiap poin ≥ 1 evidence; FakeNotifier mencatat pengiriman; jadwal 06.45/16.00 terdaftar.
- Approve Action `send_wa` → terkirim via FakeTransport; reject → suppress 14 hari (test); 3× "tidak sesuai kebijakan" → LearnedRule.
- Mesin memanggil `/decision` → 403.
