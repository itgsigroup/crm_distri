# Stage 11 — Cash engine & Collection agent

## Tujuan
Layar Kas dan agen penagihan: Won→Lunas vs benchmark, umur piutang, prediksi kas 30 hari dari pola bayar, tindakan penagihan bernada tepat, KPI lead→cash/DSO untuk Denyut bisnis.

## Baca dulu
`00` (segmen L2C, termin, pemerintah), `02` (L2C/CashItem), `06` (Collection), `05-ui-spec` (Kas).

## Kerjakan
1. L2C (dari tahap 08) → `days_in_stage` vs benchmark Policy; `blocked_reason` dari grup project (ekstraksi).
2. Pola bayar per akun (median hari bayar; `tepat|lambat|termin_anggaran`).
3. Prediksi kas 30 hari (probabilitas × nilai; DP setelah PO, termin setelah BAST) + histori akurasi; ekspor CSV untuk budget pembelian mingguan.
4. Collection agent harian: `create_invoice` (BAST clear > benchmark), `payment_reminder` (nada dari pola; pemerintah ≥ 30 hari → tanya dokumen SPM), `schedule_bast`, `request_referral` (H+3 BAST). Eksekusi lewat 07/09/10.
5. KPI lead→cash, DSO, Won→invoice, piutang jatuh tempo → Denyut bisnis. UI Kas sesuai mockup.

## Acceptance criteria
- Fixture 6 project → stage/hari seperti mockup; Panti Rapih `create_invoice`, Semen Mitra pengingat ramah, Magelang `ask_spm_documents`, Cakra `schedule_bast`.
- Prediksi kas ≈ Rp 2,4 M; what-if invoice Panti Rapih menaikkan prediksi.
