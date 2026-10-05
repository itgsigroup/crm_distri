# Stage 01 — Domain & metrics engine (rumus Orbit) + API baca dealer
**Baca**: `01-glossary.md` (seluruhnya), `03-data-model.md`, `07-api.md` (bagian Dealer, Orbit, Segmen, KPI).

## Tujuan
Semua angka Orbit dihitung deterministik di Go dan tersedia lewat API; seed menghasilkan nilai persis mockup.

## Deliverables
1. `internal/domain`: tipe `Dealer`, `DealerMetrics` (rhythm, last, cyc, due_in, status, activity, freq, avg_order, omzet_bln, segment, sow, sow_source, mix, mix_cats[6], credit{room,state,pay_days,on_time}, pic_active, score, score_parts), `Signal`, `Order`, `Invoice`, `Payment`, `StockItem`, `Policy*`.
2. `internal/metrics`: fungsi murni `Compute(d DealerHistory, p PolicySet, today time.Time) DealerMetrics` + helper (`Rhythm`, `Status`, `Segment`, `CreditState`, `Score`, `PushCandidates`, `OrderRecommendation`). Semua ambang dari `PolicySet`.
3. Tabel uji dari glossary (**semua baris**) + uji properti: skor selalu 0–100; status monoton terhadap cyc; segmen stabil di ambang.
4. Job `metrics.recompute` (river): per dealer saat ada sinyal baru; `metrics.snapshot` harian 00:30 WIB → `dealer_metrics_daily`; `dealers.metrics_current` diperbarui.
5. Endpoint: `GET /dealers`, `/dealers/{id}` (+ `/orders`, `/mix`, `/credit`, `/contacts`, `/commitments`, `/timeline`, `/memo`), `/orbit`, `/orbit/summary`, `/orbit/movers`, `/segmen`, `/segmen/summary`, `/segmen/movers` (movers memakai snapshot 90 hari lalu; bila belum ada snapshot, gunakan `db/seed/metrics_prev.json`), `/dealers/due`, `/dealers/drift`, `/dealers/credit-tight`, `/kpi`, `/agenda`, `/stock/aging`, `/stock/critical`, `/credit/*`.
6. `arc ctl metrics --dealer <id>` mencetak metrik + 5 komponen (alat debugging Sam).
7. Uji regresi seed: setelah `seed` + `recompute`, metrik 18 dealer = tabel di glossary / mockup (toleransi 0).

## Acceptance
- `go test ./internal/metrics/...` memuat ≥ 16 kasus glossary, hijau.
- `GET /api/orbit` mengembalikan 18 dealer dengan status: 8 Key account, 6 Aktif, 3 At risk, 1 Churn (sesuai mockup); `GET /api/segmen/summary` → A 7 · B 3 · C 4 · D 3 · Baru 1.
- `GET /api/dealers/{mitra}` → status At risk, segment C, credit over limit, score 44.
- `make check` hijau.

Commit: `feat(stage-01): metrics engine orbit + api baca dealer`
