# Stage 05 — LLM provider + Agen v1 (AI Order, AI Follow-up, AI Kredit) + proposal & keputusan
**Baca**: ADR 0003, `05-agents.md` (kontrak, agen 1–3, matriks), `03-data-model.md` (proposals, outbox, calibration), `07-api.md` (proposals/decide), mockup ActionSheet & kartu Keputusan.

## Tujuan
Tiga agen pertama menghasilkan proposal berprovenance dari data nyata/seed; manusia memutuskan lewat ActionSheet; keputusan `approve` masuk outbox; `reject` dengan alasan masuk kalibrasi. Belum ada Orchestrator penuh — pakai runner sederhana `agents.RunAll(scope)` yang nanti dibungkus Orchestrator (Stage 06).

## Deliverables
1. `internal/llm`: `Provider`, `anthropic`, `openai`, `fake` (fixture), `Router` (policy.llm), `mask.go` (PII), `prompts/system.md` + prompt per agen (`.md` berversi), logging `llm_calls` dengan biaya estimasi.
2. `internal/agents`: `Agent` interface, `Input` builder (`internal/agents/input.go`, membaca view + 20 sinyal terbaru masked + few-shot dari calibration), agen `order`, `followup`, `credit` sesuai 05 (logika Go dulu, LLM untuk teks + confidence, validasi JSON schema, fallback template).
3. Tabel `proposals` terisi; validasi domain: `signal_ids` ≥ 1, `confidence` 0–1, `kind ∈ Kinds()`, `autonomy` dari matriks (Stage 06 akan mengevaluasi ulang; sementara semua `approve` kecuali `so_draft` lengkap).
4. API: `GET /proposals`, `/proposals/{id}`, `POST /proposals/{id}/decide` (approve → outbox `wa` dengan preview, atau `odoo_so_draft` bila `ODOO_WRITE=true`; edit → `edited_payload`; reject → alasan enum + `calibration_events` supresi 14 hari), `GET /dealers/{id}/next`.
5. Frontend: `ActionSheet` lengkap (persis mockup: provenance, kenapa sekarang, yang disiapkan + preview, setelah disetujui, tolak dengan alasan, edit dulu), tombol aksi di Pusat kendali (Keputusan, Jadwal order, Lewat jadwal, Limit tipis), Dealer (Langkah berikutnya), Chat (konteks). Toast + optimistic update + SSE `proposal_changed`.
6. `arc ctl agents run --agent followup --dealer <id>` untuk debugging.
7. Uji per agen (05) dengan `llm.Fake`; uji decide: approve → outbox pending → `outbox.send` (wa fake) → `executed`; reject → proposal serupa dalam 14 hari `suppressed` (uji di runner).

## Acceptance
- Dengan `LLM_PROVIDER=fake`: `arc ctl agents run --all` pada seed menghasilkan ≥ 8 proposal termasuk: `credit_release` Graha (DP 50%), `followup` Prima/Jaya/Borneo (H-1), `price_counter` Indo Vision (margin < floor → approve), `so_draft` Toko Sinar (auto).
- Setujui `followup` Prima di UI → outbox `sent` (fake) ≤ 5 dtk, badge berubah, `audit_log` mencatat user.
- Tolak `push`/`followup` dengan "Tidak sesuai kebijakan" → tercatat di kalibrasi dan muncul di Pengaturan → Kalibrasi.
- Dengan `ANTHROPIC_API_KEY` nyata (opsional): 1 proposal nyata dihasilkan untuk dealer seed, biaya tercatat di `llm_calls`.
- `make check` hijau.

Commit: `feat(stage-05): llm provider, agen order/follow-up/kredit, proposal & keputusan`
