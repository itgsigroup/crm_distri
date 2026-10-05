# Stage 10 — Memori dealer, Ringkasan Orchestrator, kalibrasi & belajar, Tanya (⌘K)
**Baca**: `04-orchestrator.md` (Belajar), `05-agents.md` (few-shot), `07-api.md` (brief, ask, calibration), `09` (masking), mockup: Memori dealer, Ringkasan, Pengaturan → Kalibrasi agen, command bar.

## Tujuan
Sistem "ingat" per dealer (brief hidup berprovenance), menulis ringkasan pagi yang bisa diverifikasi, belajar dari keputusan, dan menjawab pertanyaan bebas dengan sumber.

## Deliverables
1. `internal/memory`: writer memo dealer (LLM, maks 120 kata, setiap kalimat dipetakan ke `signal_ids`; disimpan `dealers.memo` + `memo_signal_ids`); diperbarui di tahap Belajar bila ada sinyal baru dealer itu; UI menampilkan "semua klaim bisa dilacak" dengan hover per kalimat → sumber.
2. Ringkasan Orchestrator (`/brief/today`): 4 poin (Order tepat jadwal · Lewat jadwal · Over limit · Push stok) ditulis LLM dari counters + daftar kandidat, setiap poin dengan `signal_ids`; fallback template.
3. Belajar: `calibration_events` → supresi (sudah), few-shot per agen dari proposal `edited` (diff preview), `agent_state.confidence`, "pelajaran" berbahasa manusia (`/calibration` → 10 terbaru, mis. "AI Stok tidak lagi menawarkan HDD ke dealer tier C (ditolak 4×)") — dihasilkan dari agregasi penolakan (≥ 3 penolakan dengan alasan sama → 1 pelajaran).
4. `POST /ask {q}`: jawaban dari data (router: pertanyaan tentang risiko/stok/kas → query terstruktur + LLM merangkai; nama dealer → buka dealer) dengan provenance; command bar menampilkan Sheet jawaban.
5. Frontend: Memori dealer dengan provenance hover, Ringkasan dengan tautan, Pengaturan → Kalibrasi agen (bar confidence + pelajaran), Sheet jawaban ⌘K.
6. Uji: memo memiliki provenance untuk setiap kalimat (validator menolak kalimat tanpa sumber); agregasi pelajaran; ask router 5 kasus.

## Acceptance
- Setelah siklus pada seed: memo Mitra Jaya ≈ isi mockup (menyebut tempo, over limit, lewat jadwal) dan tiap kalimat punya sumber yang bisa diklik.
- Tolak 3× proposal HDD tier C dengan alasan sama → pelajaran muncul di Kalibrasi dan proposal serupa tidak muncul 14 hari.
- ⌘K "dealer mana yang berisiko" → jawaban 3 dealer dengan sumber.
- `make check` hijau.

Commit: `feat(stage-10): memori dealer, ringkasan, kalibrasi, tanya`
