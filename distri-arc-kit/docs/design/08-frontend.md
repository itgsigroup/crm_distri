# 08 · Frontend — React 19 · TypeScript · Vite

Spesifikasi visual = `reference/distri-arc-orbit-v2-mockup.html`. Buka di browser; token CSS, komponen, teks UI, dan perilaku di sana adalah kontrak. Bila doc ini dan mockup berbeda, **mockup menang** untuk visual; doc ini menang untuk data dan perilaku teknis.

## Stack
`react 19` · `react-router 7` (data router) · `@tanstack/react-query 5` · CSS Modules + `web/src/styles/tokens.css` (disalin dari `:root` mockup, termasuk dark mode) · `three ^0.160` (Peta relasi 3D; fallback canvas 2D bila WebGL gagal) · `vitest` + `@testing-library/react` · `playwright`. Tanpa UI kit; komponen ditulis sendiri mengikuti mockup (ukuran, radius, bayangan). Ikon: sprite SVG dari mockup (`web/src/icons/sprite.svg`).

## Struktur
```
web/src/
  app/            router, providers (QueryClient, SSE), shell (Rail, Topbar, Dock, Tabbar)
  api/            client fetch + tipe (di-generate dari OpenAPI yang dihasilkan backend, `npm run gen:api`)
  features/
    control/      Pusat kendali: OrchestratorCard, PlanList, DecisionQueue, BriefCard, DueList, DriftList, KpiWheels, AgendaSales, PushList, CreditTightList
    orchestrator/ OrchHeader, Pipeline, Conflicts, CycleHistory, AgentCards, McpPanel, AutonomyMatrix
    chat/         ThreadList, Thread, ContextPane, Identify
    orbit/        OrbitBoard (SVG), OrbitSummary, Movers
    segmen/       SegmenChart (SVG log-y), SegmenSummary, Plays, Movers
    relasi/       RelasiMap3D (three), Filters, Pairs, Insights
    dealer/       DealerList, DealerHeader, NextStep, OrderToCash, SowMix, Credit, Memo, Contacts, Commitments, Timeline
    stock/        AgingList, Critical, SalesByProduct
    credit/       Overview, DealerCredit, Exposure, Forecast
    settings/     Policies, Connections, AiConnection, Calibration
    guide/        Panduan (konten statis dari panduan-orbit.html, dibungkus komponen)
  components/     Card, Pill, Btn, Seg, Switch, Sheet (ActionSheet), Toast, Tip, RowList, Avatar, Ring, Bar
  lib/            format (fmtRp, tanggal WIB), sse, hooks (useCycle, useProposals…), i18n (id-ID copy)
  styles/         tokens.css, base.css
```

## Rute
`/` Pusat kendali · `/orchestrator` · `/chat/:threadId?` · `/orbit` · `/orbit/segmen` · `/orbit/relasi` · `/dealer/:id?` · `/stok` · `/kredit` · `/pengaturan` · `/panduan`. View tabs Orbit/Segmen/Peta relasi di topbar (seperti mockup). Deep link dealer dari mana pun (`data-go` di mockup = `navigate`).

## Komponen kunci (perilaku)
- **ActionSheet**: dibuka dari tombol aksi di mana pun (`proposal_id`); menampilkan agen, provenance (signal chips yang bisa diklik → Chat/Timeline), *Kenapa sekarang*, *Yang sudah disiapkan* + preview WA, *Setelah Anda setujui*, tombol **Setujui & jalankan** / **Edit dulu** (textarea preview) / **Tolak** (alasan enum + teks) / **Nanti**. Setelah keputusan: optimistic update, toast, `proposal_changed` dari SSE mengonfirmasi.
- **OrchestratorCard / Dock**: membaca `useCycle()` (query + SSE `cycle_stage`); tombol *Analisis ulang* → `POST /cycles` dengan scope dari rute aktif; saat `running` tombol disabled, pipeline menyala per tahap, progress bar; `409` → toast "Orchestrator sedang berjalan". Dock tampil di semua rute kecuali `/`, `/orchestrator`, `/chat`.
- **Command bar (⌘K)**: parser ringan: `analisis ulang <dealer|semua|stok|kredit|orbit|segmen> [lewat mcp]` → `POST /cycles`; nama dealer → buka dealer; selain itu → `POST /ask` (jawaban dengan provenance) ditampilkan di Sheet.
- **OrbitBoard**: SVG `viewBox 800×740`; lingkar Key account 92 / Aktif 170 / At risk 248 / Churn 310; sudut = `min(cyc,1)·2π` searah jarum jam dari atas; radius dalam lingkar diinterpolasi (lihat `renderOrbit` di mockup); ukuran titik `7+√sow·1.3`; warna = credit_state; label dengan tabrak-hindar; tooltip; klik → dealer; filter sales.
- **SegmenChart**: X linear 0–3,5 order/bln (sumbu ganda: ritme hari), Y log 4 jt–200 jt; garis ambang dari `policies`; zona tint; titik putus = snapshot 3 bulan lalu; **posisi titik tidak digeser** (hanya label yang dicarikan tempat); klik zona → filter + panel *Yang dilakukan* + daftar dealer dengan tombol aksi.
- **RelasiMap3D**: port engine `createNet` dari mockup ke modul TS (three r160 API); node sales (biru), dealer (warna skor); ukuran = interaksi/bulan; periode 30–180; klik dua kali → dealer; fallback 2D.
- **PlanList**: item dengan jam, agen, pill otonom/butuh approve, status; tombol aksi sesuai proposal; `data-run` untuk item auto yang belum jalan → `POST /proposals/{id}/decide approve`.
- **Chat**: 3 panel; tab Semua/Dealer/Grup internal/Nomor baru; thread virtualized; anotasi agen di bawah pesan; konteks dealer (skor ring, sisa limit, langkah berikutnya, ekstraksi); kirim balasan = keputusan manusia.
- **Settings/AiConnection**: segmented API AI / MCP / Keduanya → `PUT /policies/llm`; kartu koneksi dengan status dari `/connections`; endpoint MCP + salin; tool list; aturan.

## Data & realtime
- Satu `EventSource('/api/events')` di `SseProvider`; map event → `queryClient.invalidateQueries`.
- Query keys: `['cycle','latest']`, `['plan',date]`, `['proposals',{status}]`, `['dealer',id,...]`, `['orbit',sales]`, …
- Mutasi keputusan: optimistic + rollback.
- Format: `fmtRp` persis seperti mockup (`Rp 62 jt`, `Rp 1,28 M`), tanggal `id-ID`, angka `tabular-nums`.

## Desain & aksesibilitas
- Token: salin blok `:root` + dark mode dari mockup; font Plus Jakarta Sans / Instrument Sans / JetBrains Mono (self-host di `web/public/fonts` untuk VPS tanpa internet).
- Semua tombol ikon punya `aria-label`; fokus terlihat; `prefers-reduced-motion` mematikan animasi pipeline/3D auto-rotate.
- Responsif: ≥ 1180 (tiga kolom), 900–1180 (dua), < 900 (satu kolom + tabbar bawah) — breakpoint persis mockup.
- Tidak ada teks hardcoded di komponen: semua copy di `lib/i18n/id.ts` (sumber: mockup), istilah mengikuti glossary.

## Uji
- vitest: `fmtRp`, parser command bar, `OrbitBoard` geometri (sudut/radius untuk 5 kasus), `SegmenChart` skala log, reducer SSE → invalidation.
- Playwright (terhadap `make dev` + seed): login → Pusat kendali memuat 8 plan item → setujui 1 proposal → status berubah → Orchestrator menampilkan siklus → Analisis ulang dealer Mitra Jaya → siklus baru muncul; Orbit, Segmen, Dealer, Chat memuat tanpa error konsol.
