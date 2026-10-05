# Stage 02 — Frontend shell + Pusat kendali (baca) + Orbit/Segmen/Dealer
**Baca**: `08-frontend.md`, mockup (`reference/distri-arc-orbit-v2-mockup.html`), `07-api.md`.

## Tujuan
Aplikasi React dengan shell persis mockup (rail, topbar, command bar, tabbar mobile, dock placeholder), dan layar **Pusat kendali (bagian baca)**, **Orbit**, **Segmen**, **Dealer** memakai API Stage 01. Belum ada proposal/keputusan nyata (kartu Keputusan dan Rencana menampilkan empty state yang didesain).

## Deliverables
1. `web/src/app`: router (semua rute di 08), `Shell` (Rail, Topbar + view tabs, Tabbar), `QueryProvider`, `SseProvider` (koneksi `/api/events`, belum ada event → heartbeat saja).
2. `components/`: Card, Pill, Btn, Seg, Switch, Toast, Tip, RowList, Avatar, Ring, Bar, Sheet (kerangka), Kpi tiles — gaya dan ukuran dari mockup, token dari `tokens.css`.
3. `features/orbit/OrbitBoard` (SVG, geometri persis mockup termasuk label anti-tabrak), `OrbitSummary`, `Movers`; `features/segmen/SegmenChart` (log-Y, ambang dari `/policies` — sementara konstanta), `SegmenSummary`, `Plays`, `Movers` dengan interaksi klik zona.
4. `features/dealer/*` lengkap (header + 4 KPI, anchors sticky, Order-to-cash ring, Share of wallet & product mix, Sisa limit, Memori, PIC aktif, Komitmen, Timeline); `DealerList` dengan pencarian trigram (server).
5. `features/control/*` bagian baca: Brief (sementara dari `/brief/today` yang mengembalikan template dari metrik), DueList, DriftList, KpiWheels, AgendaSales, PushList (dari `/stock/push`), CreditTightList. Tombol aksi menampilkan Sheet "Belum ada proposal — menunggu Orchestrator (Stage 06)".
6. `lib/format` (`fmtRp` dsb.), `lib/i18n/id.ts` (semua copy dari mockup), `icons/sprite.svg`.
7. vitest: fmtRp, geometri OrbitBoard (5 kasus), skala SegmenChart; playwright smoke: 6 rute memuat tanpa error konsol.

## Acceptance
- `make dev` → `http://localhost:5173` menampilkan Pusat kendali dengan angka dari API (6 jadwal order, 4 lewat jadwal, 3 limit tipis), Orbit 18 titik dengan status benar, Segmen 4 zona, Dealer Mitra Jaya menampilkan skor 44 dan 5 komponen.
- Tampilan desktop 1440 dan mobile 390 sesuai mockup (screenshot playwright disimpan di `web/e2e/__screenshots__`); tidak ada scroll horizontal.
- Dark mode mengikuti sistem.
- `make check` hijau.

Commit: `feat(stage-02): react shell, pusat kendali (baca), orbit, segmen, dealer`
