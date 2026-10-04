# Stage 05 — Pipeline & deal intelligence (ARC-native) + Prospek inbound + Action sheet

## Tujuan
Dari chat menjadi pipeline: opportunity dibuat dari percakapan (usulan agen, approve manusia), kanban dengan stage seed Odoo, medan health × nilai, health/sinyal/flag, saran tindakan dengan action sheet lengkap (approve/edit/tolak dengan alasan → kalibrasi), dan view Prospek untuk nomor masuk.

## Baca dulu
`02` (health, stage & sinyal, Action), `06` (Follow-up, Meeting prep, Forecast, Research), `04` (batas otonomi), `05-ui-spec` (Penjualan, Prospek, action sheet).

## Kerjakan
1. Opportunity dari percakapan: Capture/Research mengusulkan Action `create_opportunity` (nama, akun, nilai estimasi, stage `Baru`, evidence) bila ada permintaan penawaran/spesifikasi; approve → Opportunity ARC dibuat (source_system null). Pindah stage manual di kanban (drag) dengan audit; `signal` sub-status dari bukti.
2. `score_opportunities` (harian + segera saat ada interaksi baru): health & breakdown, histori 30 hari; detektor sinyal deterministik (`silent`, `single_threaded`, `po_overdue`, `commitment_late`, `champion_moved`, `competitor_mentioned`).
3. NBA rules `rules/nba.yaml` (kasus mockup wajib) → satu Action aktif per opportunity; Follow-up agent draft WA/email gaya pemilik kanal (preview); Meeting prep dari kalender manual/entri (Google Calendar baru di tahap 09).
4. Forecast v1: commit/best/pipeline vs target (Policy), weighted ARC vs manual, what-if.
5. Action lifecycle lengkap: `POST /actions/{id}/decision {approve|edit|reject|snooze, reason, note}` role human, audit, executor (send_wa via transport setelah approve; create_task → tabel Task ARC + placeholder Basecamp), kalibrasi (acceptance rate per agen/tipe, suppress 14 hari, LearnedRule dari 3× "tidak sesuai kebijakan").
6. Prospek: funnel event per InboundContact (masuk → teridentifikasi → relevan → pain point tergali → lead → penawaran → won), `GET /funnel?month=`, aksi `create_lead` (= create_opportunity dengan konteks & pertanyaan), `reply_first_question`, `mark_not_prospect` (reversible).
7. UI: Penjualan/Pipeline (bar "Odoo belum terhubung — stage ARC", kanban dengan kartu + baris ARC + langkah berikutnya, Medan ARC, panel weighted, pola win/loss dari data), Penjualan/Prospek (funnel, nomor baru masuk + detail identifikasi/overview/solusi/pertanyaan), action sheet lengkap (provenance line, kenapa, yang disiapkan, langkah, tombol, tolak dengan alasan), Hari ini → Keputusan dengan filter & item terlipat, Relasi → Langkah berikutnya & Deal intelligence.
8. Playwright: WA masuk (Fake) → Action `create_opportunity` → approve → kartu di kanban → health & flag terlihat → tolak satu saran dengan alasan → kartu berubah & kalibrasi tercatat.

## Acceptance criteria
- Seed fixture → health & flag 8 opportunity cocok mockup ± 5; forecast commit Rp 4,3 M / best 5,9 / pipeline 11,9; what-if BSD → 3,4.
- Aturan NBA tabel-driven ≥ 10 kasus; tidak ada Action yang mengirim tanpa approve (test).
- Funnel fixture 7 tahap; Rudi bisa dibuat lead dari UI dengan pertanyaan terlampir.
- Playwright lolos.
