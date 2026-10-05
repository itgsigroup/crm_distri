# 00 · Overview — Distri ARC Orbit

## Apa ini
CRM AI-native untuk distribusi B2B (GSI menjual CCTV, HDD, kabel/PoE, modul LED, fire alarm, aksesoris ke dealer/toko/installer dengan termin kredit). Dealer yang sama order berulang; tidak ada "closing". Yang dikelola adalah **orbit**: apakah dealer order sesuai siklusnya, apakah limit kreditnya sehat, apakah product mix-nya melebar, apakah relasinya cukup kuat.

## Tiga pandangan untuk satu dealer
| Pandangan | Pertanyaan | Layar |
|---|---|---|
| **Orbit** | *Kapan* dealer harus di-follow-up? (posisi dalam siklus order) | Orbit |
| **Segmen** | *Dealer jenis apa* ini? (seringnya × besarnya order → A/B/C/D) | Segmen |
| **Peta relasi 3D** | *Seberapa dekat* sales dengan dealer? (interaksi WA + order) | Peta relasi |

**Skor dealer** (0–100) merangkum ketiganya; segmen memilih *cara melayani*, orbit memilih *hari menyapa*.

## Lapisan sistem
```
 WhatsApp (whatsmeow / Cloud API)   Odoo (SO · invoice · bayar · stok)   Getcontact (manual)
            │ sinyal                         │ sinyal                         │
            ▼                                ▼                                ▼
      ┌──────────────────────────── signals (Postgres) ────────────────────────────┐
      │                                                                            │
      │   metrics (Go, deterministik): siklus order · jadwal · status · segmen ·   │
      │   share of wallet · product mix · sisa limit · PIC aktif · skor dealer     │
      │                                                                            │
      │   ORCHESTRATOR (worker, tiap jam + on-demand + MCP)                        │
      │   Ingest → Analisis (6 agen paralel) → Sintesis & konflik → Keputusan →    │
      │   Eksekusi (outbox, hanya yang disetujui) → Belajar (kalibrasi)            │
      └────────────────────────────────────────────────────────────────────────────┘
            │ REST + SSE                                   │ MCP (Streamable HTTP)
            ▼                                              ▼
      React SPA (Pusat kendali, Orchestrator, Chat,    Claude Desktop · ChatGPT · agent
      Orbit/Segmen/Peta relasi, Dealer, Push stok,      eksternal: baca · analisis ·
      Kredit & kas, Pengaturan, Panduan)                orkestrasi (tanpa kirim)
```

## Prinsip desain
1. **Orchestrator-first.** Satu pengatur di atas enam agen. Agen = fungsi murni `Analyze(input) → proposals`; Orchestrator yang membaca sinyal, membagi tugas, menyelesaikan konflik, memutuskan otonomi, menjalankan, dan belajar.
2. **Manusia memutuskan.** Keputusan adalah tabel (`proposals` dengan status), bukan tombol. Tidak ada jalur ke dealer selain outbox yang diisi oleh proposal `approved`.
3. **Angka dihitung, bukan dikarang.** Semua metrik Orbit dari `internal/metrics` (Go, tabel kasus uji). LLM menulis alasan dan draft, memilih antar opsi, tidak menghitung.
4. **Provenance di mana-mana.** Setiap proposal, brief, dan memori dealer menunjuk `signal_ids`. UI menampilkan sumber di tiap klaim.
5. **Kebijakan sebagai data.** Ambang (1,2× siklus, Rp 20 jt, 40% ruang, floor margin 9%), matriks otonomi, dan kebijakan MCP hidup di tabel `policies` (JSONB, berversi), diedit di Pengaturan, dibaca Orchestrator tiap siklus.
6. **Odoo source of truth.** Distri ARC tidak menduplikasi ERP; ia membaca Odoo, menghitung, mengusulkan, dan menulis SO draft dengan catatan sumber.
7. **Satu bahasa.** Istilah UI persis mengikuti `01-glossary.md`.

## Layar (lihat mockup)
| Layar | Isi | Sumber data |
|---|---|---|
| Pusat kendali | Kartu Orchestrator (status, pipeline, counters), Rencana hari ini, Keputusan, Ringkasan, Jadwal order 7 hari, Lewat jadwal, KPI utama, Agenda per sales, Push stok, Limit tipis | `/cycles/latest`, `/plan/today`, `/proposals?status=proposed`, `/dealers/due`, `/dealers/drift`, `/kpi`, `/agenda` |
| Orchestrator | Status, jalur analisis, pipeline, resolusi konflik, riwayat siklus, kartu agen, MCP sebagai orchestrator, matriks otonomi | `/cycles`, `/cycles/{id}`, `/conflicts`, `/agents`, `/mcp/calls`, `/policies/autonomy` |
| Chat | WA 3 panel: daftar (dealer, grup internal, nomor baru), thread, konteks dealer + ekstraksi | `/chat/threads`, `/chat/threads/{id}`, SSE |
| Orbit | Papan orbit (SVG), isi orbit per status, yang bergerak | `/orbit` |
| Segmen | Scatter X seringnya × Y besarnya, isi segmen, yang dilakukan, pindah segmen | `/segmen` |
| Peta relasi 3D | Graph sales ↔ dealer (three.js), pasangan terkuat, pola relasi | `/relasi?period=30` |
| Dealer | Header KPI, Langkah berikutnya, Order-to-cash, Share of wallet & product mix, Sisa limit, Memori, PIC aktif, Komitmen, Timeline | `/dealers/{id}` + sub-resources |
| Push stok | KPI stok, kandidat push, stok kritis, penjualan per produk | `/stock/aging`, `/stock/critical` |
| Kredit · kas | KPI kas, sisa limit tiap dealer, exposure vs limit, prediksi kas masuk | `/credit/overview`, `/credit/forecast` |
| Pengaturan | Kebijakan orbit, sumber sinyal, Koneksi AI (API & MCP), kalibrasi agen | `/policies`, `/connections`, `/calibration` |
| Panduan | Konsep Orbit (konten statis dari `reference/panduan-orbit.html`) | — |

## Yang sengaja tidak dibangun
- Tidak ada pipeline/stage yang dipindah tangan.
- Tidak ada pengiriman otomatis ke dealer dalam bentuk apa pun.
- Tidak ada akuntansi; angka kas dari Odoo.
- Tidak multi-tenant; satu instalasi per perusahaan.
