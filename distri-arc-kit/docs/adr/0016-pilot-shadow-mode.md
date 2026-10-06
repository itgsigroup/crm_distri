# ADR 0016 — Pilot cabang: mode bayangan, pengukuran, buka otonomi bertahap
**Status**: diterima · 2026-10-06 · melengkapi ADR 0008 (penjaga otonomi) dan 0014 (outbox)

## Keputusan
1. Kebijakan baru `pilot` (berversi, CEO): `mode` off | shadow | live, `branch`, `started_at`, `shadow_days` (14),
   `unlock_confidence` (80), `unlock_weeks` (2), `min_decisions_per_week` (5), `unlocked` (agen yang dibuka).
2. **Mode bayangan ditegakkan di empat lapis**, bukan hanya di UI: (a) `Evaluate` Orchestrator tidak memberi `auto`;
   (b) `Decide` mencatat keputusan & kalibrasi tetapi tanpa job, baris outbox menjadi `shadow`, gelembung chat pending
   dihapus, catatan Odoo tidak dibuat; `ApproveBySystem` menolak (`ErrShadow`); (c) balasan dari layar Chat ditolak
   409 (sales membalas dari ponsel); (d) `Sender.Send` tidak pernah mengirim baris `shadow` dan menahan baris yang
   antre sebelum mode berubah. Alert sistem (`wa_system`) tetap jalan — internal.
3. **Mode live**: kirim hanya setelah keputusan manusia; langkah otomatis hanya untuk agen di `unlocked`, dan tetap
   tunduk pada matriks otonomi + penjaga ADR 0008. Membuka agen (`POST /pilot/unlock`, CEO) dicek server: dua minggu
   penuh terakhir berturut-turut, masing-masing ≥ `min_decisions_per_week` keputusan manusia dan ≥ `unlock_confidence`%.
4. **Pengukuran** (`internal/pilot`, SQL + `views.KPI`, tanpa LLM): per agen (saran, setuju, edit, tolak, ditunda,
   terbuka, otonom, % diterima, median menit keputusan, confidence mingguan); order tepat jadwal & DSO cabang; lewat
   jadwal tertangkap sebelum churn (dealer pernah *At risk* di periode → ada proposal diputuskan sesudahnya → order lagi
   → tidak menjadi *Churn*). Saran tanpa dealer (transfer stok, PO) dihitung untuk cabang mana pun.
5. **Audit pilot** (0 = lolos): kirim tanpa keputusan tercatat, kirim selama bayangan, alert ke selain grup internal,
   DM internal tersimpan, pesan grup internal tertaut dealer, identifikasi nomor outbound. Tersedia di layar, CSV, dan
   `arc ctl pilot audit` (exit 1).
6. **Snapshot mingguan** `pilot_weeks` (job `pilot.snapshot` Senin 00.45 WIB, idempoten per minggu & cabang) dan CSV
   mingguan. **Konfirmasi share of wallet** massal per kuartal (`dealer_sow_estimates`, sales hanya dealernya) memicu
   hitung ulang metrik dealer itu.

## Konsekuensi
- Pilot yang sebenarnya (data Odoo Semarang, 2 nomor WA, 14 hari, tanda tangan Sam) dijalankan GSI; laporan di
  `docs/PILOT-REPORT.md`. Stage 14 berstatus `in-progress` sampai laporan ditandatangani.
