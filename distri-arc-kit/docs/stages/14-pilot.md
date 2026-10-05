# Stage 14 — Pilot 1 cabang: migrasi data nyata, pelatihan, pengukuran, iterasi
**Baca**: `00-overview.md`, `10-testing-ops.md`, semua OPEN-QUESTIONS.

## Deliverables
1. Migrasi: `arc ctl odoo sync --full` cabang pilot (Semarang), pairing WA 2 nomor sales, impor `internal_numbers` & `wa_groups`, konfirmasi SOW oleh sales untuk 20 dealer teratas (UI massal di Dealer → *Konfirmasi share of wallet*).
2. Mode **bayangan** 2 minggu: Orchestrator berjalan, semua proposal `approve` (tidak ada auto), tidak ada kirim; Sam & sales menilai proposal (setujui/tolak) → kalibrasi.
3. Ukur: % proposal disetujui per agen, waktu keputusan, order tepat jadwal, DSO, lewat jadwal yang tertangkap sebelum churn — dashboard `Pengaturan → Pilot` + ekspor CSV mingguan.
4. Buka otonomi bertahap sesuai `autonomy.matrix` setelah confidence agen ≥ 80% dua minggu berturut.
5. Dokumen: `docs/PILOT-REPORT.md` (temuan, perubahan kebijakan, rekomendasi cabang berikutnya), pembaruan `panduan-orbit` bila istilah berubah.

## Acceptance
- 2 minggu data nyata tanpa insiden privasi (audit) dan tanpa kirim yang tidak disetujui (outbox = proposal approved 100%).
- Laporan pilot ditandatangani Sam; keputusan go/no-go cabang kedua.

Commit: `feat(stage-14): pilot cabang semarang`
