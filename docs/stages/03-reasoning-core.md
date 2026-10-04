# Stage 03 — Reasoning core: ekstraksi, identitas, memori

## Tujuan
Agen yang menganalisis setiap pesan masuk: provider LLM dengan routing & masking, ekstraksi komitmen/sinyal/sentimen/tugas dengan provenance, identifikasi nomor baru (siapa & dari mana) beserta overview, memori akun, hygiene. Setelah tahap ini, pesan WA yang masuk sudah "dipahami".

## Baca dulu
`06-agents.md` (Capture, Identity, Research, Hygiene), `03-integrations.md` (LLM, identifikasi nomor), `04` (privasi, PII).

## Kerjakan
1. `packages/core/llm/`: `LLMProvider.complete_json(schema, messages, tier)`; `AnthropicProvider`, `OpenAIProvider`, `OllamaProvider`, `FakeProvider` (deterministik dari fixture). Router tier (`light/heavy/interactive`) dari config; retry, timeout, fallback; log `LLMCall` (token, biaya estimasi, hash).
2. `mask_pii/unmask` (nomor +62/08, email, rekening) — diuji.
3. Prompt Capture v1 (`prompts/capture/v1.md`) → `commitments[]`, `signals[]`, `sentiment`, `tasks[] {text, assignee_hint}` (untuk grup), `people_mentioned[]`, `summary`. Kutipan verbatim wajib. Job `extract_interactions` per jam + `POST /jobs/extract/run` + mode segera untuk pesan baru (debounce 60 detik per thread) supaya uji coba terasa cepat.
4. Aturan per tipe: grup eksternal → ekstraksi penuh; grup internal → hanya `tasks` & jadwal; 1:1 pelanggan → penuh.
5. **Identity agent** untuk `InboundContact`: sumber berurutan — Person ARC → profil WA (nama/about via bridge) → `TruecallerProvider` (interface + Fake) → impor manual Getcontact (endpoint tempel teks tag; sumber `getcontact_manual`) → Research web (`WebSearchProvider` interface + Fake) → gabung ≥ 2 sumber → `identification{name, role, company, sources, confidence}`, `fit_score`, status `identified|unknown|not_prospect`. Overview (LLM heavy) orang & perusahaan.
6. **Research agent v1**: untuk inbound `identified` dengan fit ≥ 60 → tangga solusi (diminta → peluang → paket; estimasi dari tabel harga lini di config) dan ≤ 5 pertanyaan pain point per peran, masing-masing memetakan solusi yang dibuka. Output ke `InboundContact.solutions/pain_questions`.
7. Memori akun: `prompts/memory/v1.md` → `Account.memory` (≤ 180 kata) + history.
8. Hygiene: duplikat Person (nama+telepon fuzzy) → Action `merge_person` (proposed) bila conf < 0.9; auto-merge ≥ 0.9 dengan audit; pembersih InboundContact > 90 hari yang tidak jadi lead.
9. Eval: `tests/eval/capture_cases.yaml` (≥ 15 kasus WA dari fixture chats) + `make eval` (Fake **dan** provider asli bila key ada) → laporan precision/recall ke `docs/eval/`.

## Acceptance criteria
- Fixture chats → komitmen "revisi Senin" (kami), "PO Kamis" (mereka), sinyal `competitor_mentioned` di RSUD, tugas "sewa crane → Bayu" dari grup Simpang Lima; semua dengan evidence & confidence; ekstraksi ulang tidak menggandakan.
- Inbound fixture: Rudi → identified, company "PT Prima Karya Sejahtera", ≥ 4 solusi, 5 pertanyaan; agensi → not_prospect; nomor kosong → unknown.
- Eval ≥ 90% dengan FakeProvider. Tidak ada nomor telepon mentah di payload provider (test intercept). `GET /llm/usage` berfungsi.
