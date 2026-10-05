# Stage 09 — Agen v2: AI Stok, AI Penagihan, AI Prospek + layar Push stok & Kredit · kas
**Baca**: `05-agents.md` (agen 4–6), `01-glossary.md` (Push stok, Sisa limit, KPI), `07-api.md` (stock, credit, identify), mockup layar Push stok, Kredit · kas, Chat → Nomor baru.

## Tujuan
Tiga agen sisanya masuk Orchestrator; layar Push stok dan Kredit · kas hidup dari data; nomor baru diidentifikasi dan diusulkan jadi dealer.

## Deliverables
1. `agents/stock` (kandidat push via `metrics.PushCandidates`, bundle ≥ floor, transfer antar cabang, PO request stok kritis), `agents/collect` (H-3 ramah auto; nada tegas & cicilan approve; sinkron jadwal order), `agents/prospect` (identifikasi: profil WA Business via transport, Truecaller adapter (fake bila tanpa API), impor CSV Getcontact manual; skor; usul tier C; potensi dari dealer sejenis).
2. Orchestrator: aturan `credit_over_stock` kini punya pasangan nyata (collect); `plan.Build` memasukkan item Penagihan H-3 pagi.
3. API: `/stock/aging`, `/stock/critical`, `/stock/sales-by-product`, `/credit/overview`, `/credit/dealers`, `/credit/exposure`, `/credit/forecast` (prediksi kas masuk 30 hari = Σ invoice terbuka × probabilitas bayar dari pola bayar dealer), `/chat/identify`, `/identifications/import`.
4. Frontend: layar **Push stok** (KPI, kandidat per SKU dengan tombol aksi, stok kritis dengan *Usulkan transfer*/*Ajukan PO*, penjualan per produk 30 hari), **Kredit · kas** (KPI, sisa limit tiap dealer, exposure vs limit, prediksi kas masuk), Chat → *Nomor baru* konteks identifikasi + tombol *Buat dealer tier C* / *Kirim harga* (proposal approve).
5. Uji agen (05) + prediksi kas (fixture) + impor CSV.

## Acceptance
- Siklus pada seed menghasilkan: `push_stock` LED P5 → 6 dealer (2 jadwal order minggu ini), `collect` H-3 Nusa Teknik (auto), `installment` Mitra Jaya (approve), `new_dealer` Toko Mandiri Pati (approve).
- `GET /credit/forecast` ≈ Rp 1,02 M pada seed (±5%).
- Push stok dan Kredit · kas tampil persis mockup dengan data API.
- `make check` hijau.

Commit: `feat(stage-09): agen stok/penagihan/prospek, layar push stok & kredit`
