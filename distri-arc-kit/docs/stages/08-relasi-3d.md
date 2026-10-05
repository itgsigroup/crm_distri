# Stage 08 — Peta relasi 3D + pola relasi + PIC aktif
**Baca**: `08-frontend.md` (RelasiMap3D), `07-api.md` (relasi), `01-glossary.md` (PIC aktif), mockup layar Peta relasi 3D dan engine `createNet`.

## Tujuan
Graph sales ↔ dealer dari interaksi WA + order per periode (30–180 hari), 3D dengan three.js, fallback 2D, insight pola relasi, dan PIC aktif per dealer dihitung dari data nyata.

## Deliverables
1. SQL/`sqlc`: agregasi interaksi per (sales, dealer, bulan) dari `chat_messages` + `orders` → `GET /relasi?period=&sales=` (nodes, edges dengan bobot per bulan), `/relasi/insights` (aturan: dealer lewat jadwal dengan interaksi turun; dua sales satu dealer; order besar WA tipis).
2. `contacts.interactions_90d`, `last_interaction_at`, PIC aktif & kontak utama dihitung job harian; flag *Hanya 1 PIC* di Dealer dan proposal AI Follow-up menambahkan "minta nomor admin/kasir".
3. Frontend: `RelasiMap3D` port dari engine mockup ke TypeScript (three r160; force layout di worker web supaya UI tidak macet), kontrol periode, filter sales, pasangan terkuat, insight; klik dua kali → dealer; `prefers-reduced-motion` mematikan rotasi; fallback canvas 2D bila WebGL tidak ada; mini peta di Dealer (opsional bila waktu).
4. Uji: agregasi per periode (fixture), insight rules, vitest layout worker deterministik (seed).

## Acceptance
- `/orbit/relasi` menampilkan 4 sales + 18 dealer, ukuran node ∝ interaksi, "Pasangan terkuat" Rizky ↔ Indo Vision (104) sesuai seed; periode 180 hr mengubah angka.
- Insight: "Mitra Jaya: 30 hari tanpa order (siklus 21) — intensitas WA turun" muncul.
- Dealer Mitra Jaya menampilkan 2 PIC aktif; Toko A seed dengan 1 PIC menampilkan flag.
- `make check` hijau; e2e memuat peta tanpa error.

Commit: `feat(stage-08): peta relasi 3d, pola relasi, pic aktif`
