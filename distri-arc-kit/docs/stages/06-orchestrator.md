# Stage 06 — Orchestrator: siklus per jam, 6 tahap, konflik, otonomi, rencana, SSE, layar Orchestrator
**Baca**: `04-orchestrator.md` (seluruhnya), `05-agents.md` (matriks), `07-api.md` (Orchestrator), mockup: kartu Orchestrator, Rencana hari ini, layar Orchestrator, dock.

## Tujuan
Semua analisis lewat `Orchestrator.Run`. Siklus tercatat lengkap, konflik diselesaikan dengan aturan, otonomi dievaluasi dari kebijakan, Rencana hari ini tersusun, UI menampilkan pipeline hidup.

## Deliverables
1. `internal/orchestrator`: `Run(ctx, Scope, Trigger)`; 6 tahap sesuai 04 dengan `cycle_stages` + NOTIFY tiap transisi; advisory lock; errgroup maks 4 agen; aturan konflik (`credit_over_stock`, `collect_before_followup`, `margin_floor`, `one_owner`, `dedupe`, `suppression`, `followup_gap`) sebagai fungsi murni yang diuji; evaluasi otonomi (`policy.autonomy.matrix` + guard); `plan.Build` (maks 10 item, jam, status); tahap Eksekusi mengisi outbox hanya untuk `approved`; tahap Belajar (calibration, confidence, note).
2. Job river `cycle.run` periodic tiap jam 06–20 WIB (`unique_opts` per jam) dan `cycle.run` on-demand dari `POST /cycles` (scope, via) → `202` atau `409`.
3. API: `/cycles/latest`, `/cycles`, `/cycles/{id}`, `/conflicts`, `/agents`, `/agents/{name}/run`, `/plan/today`, `/brief/today` (dari siklus terakhir: 4 poin dengan tautan dealer dan provenance), `/policies/autonomy` (GET/PUT). SSE `cycle_stage`, `cycle_done`.
4. Frontend: `OrchestratorCard` (status, pipeline 6 chip, counters, Analisis ulang), `PlanList` (persis mockup), layar **Orchestrator** lengkap (header + jalur analisis, Pipeline, Resolusi konflik, Riwayat analisis, kartu Agen dengan *Jalankan ulang*, Matriks otonomi; panel MCP placeholder "Stage 07"), `Dock` di rute lain, tombol *Analisis ulang* di Orbit/Segmen/Push stok/Kredit/Dealer dengan scope, command bar `analisis ulang …`, `409` → toast.
5. `arc ctl reanalyze --scope dealer:<id>` dan `arc ctl cycle status`.
6. Uji 04 (semua) + uji API `409` saat siklus berjalan + uji SSE (hub) + vitest `useCycle` reducer.

## Acceptance
- `make dev` lalu tunggu/paksa `arc ctl reanalyze --scope all` (fake LLM): siklus `done` ≤ 30 dtk; `cycle_stages` 6 baris `done`; `conflicts` memuat `credit_over_stock` (Mitra Jaya) dan `collect_before_followup` (Nusa Teknik); `plan_items` 8 item: 3 auto, 5 approve (sesuai mockup).
- Di UI: tombol Analisis ulang di Orbit → pipeline menyala per tahap, pill topbar berganti nomor siklus, riwayat bertambah, toast "Analisis ulang orbit selesai …".
- Proposal confidence 0.75 tidak pernah auto; `credit_release` tidak pernah auto; dua `POST /cycles` bersamaan → satu `409`.
- `make check` hijau.

Commit: `feat(stage-06): orchestrator 6 tahap, konflik, otonomi, rencana, layar orchestrator`
