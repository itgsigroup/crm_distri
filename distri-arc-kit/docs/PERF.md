# Performa — Stage 13

Target: p95 < 300 ms per endpoint berat pada 50k sinyal. Diukur dengan `make loadtest` (`tools/loadtest`, dev saja):
data diisi sampai 50.000 sinyal (24 bulan, 4 jenis, sepertiga dengan kesimpulan agen) dan 20.000 pesan chat (85 hari),
lalu 400 request per endpoint dengan 16 klien paralel ke `bin/arc api` lokal (MacBook, PostgreSQL 17).

| Endpoint | p50 | p95 | maks |
|---|---|---|---|
| `GET /api/orbit` | 56 ms | 94 ms | 105 ms |
| `GET /api/segmen` | 73 ms | 93 ms | 144 ms |
| `GET /api/relasi` | 81 ms | 96 ms | 109 ms |
| `GET /api/dealers/sinar` (timeline) | 96 ms | 122 ms | 133 ms |
| `GET /api/dealers/due` | 73 ms | 99 ms | 128 ms |

Dengan 8 klien paralel semua p95 ≤ 61 ms. Waktu didominasi perhitungan view di Go, bukan database.

## EXPLAIN pada query terberat (2026-10-06, 50k sinyal)
| Query | Sebelum | Tindakan | Sesudah |
|---|---|---|---|
| Timeline dealer (`DealerTimelineFull`) | 1,9 ms · `Seq Scan on proposals` karena join `rp.id::text = payload->>'reply_to'` | join `rp.id = (payload->>'reply_to')::uuid` (pakai PK) + indeks parsial `signals_timeline (dealer_id, occurred_at desc) where payload ? 'conclusion' or payload ? 'reply_to'` | 0,2 ms |
| Relasi bulanan (chat 90 hari) | 13,8 ms · 21 partisi dipangkas, seq scan 3 partisi yang relevan | tidak ada: hampir semua baris di jendela dibaca untuk agregasi, seq scan paling murah | 13,8 ms |
| Ringkasan memo (`signals` dealer, 40 terakhir) | 0,5 ms · index `signals_dealer_time` per partisi | — | 0,5 ms |
| Sinyal sejak T (Ingest) | 0,1 ms · 24 partisi dipangkas | — | 0,1 ms |
| Sinyal per id (provenance) | 0,3 ms · `signals_id` di tiap partisi | — | 0,3 ms |

Partisi bulanan membuat query berjendela waktu hanya menyentuh bulan yang relevan (`Subplans Removed`), dan retensi
membuang satu bulan sekaligus (`drop table`), bukan `delete` jutaan baris.

## Menjalankan ulang
```bash
make dev                         # atau bin/arc api di :8080 dengan APP_ENV=dev
make loadtest                    # isi data + EXPLAIN + p95; keluar 1 bila p95 > 300 ms
go run ./tools/loadtest -cleanup # hapus baris uji (atau make reset)
k6 run tools/loadtest/k6.js      # versi k6 (k6 tidak wajib)
```
