# 01 · Glossary & Rumus — bahasa Orbit (wajib dipakai persis)

Semua rumus diimplementasikan di `internal/metrics` (Go murni) dan diuji dengan tabel kasus di bawah. Ambang (nilai berawalan `policy.`) dibaca dari tabel `policies`; default tertulis di sini.

## Satuan waktu
| Istilah | Definisi | Rumus |
|---|---|---|
| **Siklus order** (hari) | Jarak rata-rata antar order dealer | `median(selisih hari antar order dikonfirmasi, 6 bulan terakhir)`; butuh ≥ 2 order. Dealer 1 order → `rhythm = null` (status "baru") |
| **cyc** | Posisi dalam siklus | `hari_sejak_order_terakhir / siklus_order` |
| **Jadwal order** | Hari dealer seharusnya order lagi | `order_terakhir + siklus_order`; `due_in = jadwal - hari_ini` |
| **Lewat jadwal** | Dealer melewati 1,2× siklus tanpa order | `cyc > policy.drift (1.2)` |

## Status dealer (lingkar orbit)
Urutan evaluasi penting:
```
if rhythm == null            → "Baru"      (tampil di lingkar Aktif, titik hollow)
if cyc > 2.0                 → "Churn"
if cyc > policy.drift (1.2)  → "At risk"
if sow >= 50 && on_time >= 85  → "Key account"   (masih di dalam 1,2× siklus)
else                         → "Aktif"
```
Label UI: **Key account · Aktif · At risk · Churn**.

## Aktivitas
`normal` jika `cyc ≤ 1.05`; `menurun` jika `cyc ≤ 2`; `berhenti` jika `cyc > 2`; `baru` jika rhythm null.

## Seringnya dan besarnya order (Segmen)
| Istilah | Rumus |
|---|---|
| **Seringnya** (order/bulan) | `30 / siklus_order` |
| **Besarnya** (Rp per order) | `rata-rata nilai SO dikonfirmasi, 6 bulan` |
| **Omzet/bln** | `besarnya × seringnya` (dealer baru: nilai order pertama) |
| **Segmen** | A = sering ≥ `policy.freq (1.5)` & besar ≥ `policy.size (Rp 20.000.000)`; B = sering & kecil; C = jarang & besar; D = jarang & kecil; Baru = rhythm null |

Cara melayani (teks UI, jangan diubah): A *Prioritas: jaga & layani terbaik*; B *Upsell: naikkan nilai order*; C *Project-based: ikuti proyeknya*; D *Low-touch: layani otomatis*.

## Share of wallet (SOW)
`sow = belanja_ke_GSI / estimasi_total_belanja_kategori_GSI × 100`, integer 0–100. Estimasi total dari (a) nilai yang dikonfirmasi sales per kuartal (`dealer_sow_estimates`), (b) bila tidak ada, dari kategori kompetitor yang disebut di WA (ekstraksi AI Order, `confidence ≥ 0.7`), (c) bila tidak ada, default 50 dengan `sow_source = "default"` dan ditampilkan sebagai "estimasi".

## Product mix
6 kategori tetap: `Kamera & NVR · HDD & storage · Kabel & PoE · Modul LED · Fire alarm · Aksesoris` (mapping dari `product_category_id` Odoo di tabel `category_map`). `mix = jumlah kategori dengan ≥ 1 order 6 bulan / 6`. Kategori kosong = **peluang** untuk AI Stok.

## Sisa limit (kredit)
```
room   = 1 - exposure / limit          (dealer cash: limit = 0 → state "cash")
late   = ada invoice lewat tempo (> termin)
state  = "over limit"  jika room < 0
       = "overdue"     jika late
       = "tipis"       jika room < policy.room_min (0.40) atau pola_bayar > policy.pay_max (35 hari)
       = "aman"        selain itu
pola_bayar = rata-rata hari invoice → pembayaran, 6 bulan
on_time    = % invoice dibayar ≤ termin, 12 bulan
```
Warna: aman = good, tipis = warn, over limit/overdue = bad, cash = neutral.

## PIC aktif
Kontak di dealer yang membalas WA atau memberi order dalam 90 hari. `pic_aktif = count`. **Hanya 1 PIC** (`≤ 1`) → flag risiko. Kontak utama = PIC dengan interaksi terbanyak.

## Skor dealer (0–100)
```
r = rhythm null ? 50 : max(0, 100 - max(0, cyc - 1) × 120)
p = sow
k = mix × 100
n = limit == 0 ? 80 : clamp(room × 150 + (on_time - 50), 0, 100)
i = min(100, pic_aktif × 40)
skor = round(0.30·r + 0.25·p + 0.15·k + 0.15·n + 0.15·i)
```
Band: `≥ 70` kuat (good), `50–69` waspada (warn), `< 50` lemah (bad). UI selalu menampilkan 5 komponen, bukan hanya angka.

## Order-to-cash (fase per SO)
`Order → Siap → Kirim → Invoice → Bayar`. Mapping dari Odoo: `sale.order.state` (draft/sent = Order; sale = Siap), `stock.picking.state` (done = Kirim), `account.move.state` posted (Invoice), `payment_state` paid (Bayar). `lama_putaran = hari Order → Bayar`.

## KPI utama (per cabang dan total)
| KPI | Rumus | Target default |
|---|---|---|
| **Order tepat jadwal** | `% dealer aktif dengan cyc ≤ 1.2` | ≥ 85% |
| **DSO** | `rata-rata hari Order → Bayar, 90 hari` | ≤ 30 hari |
| **Perputaran stok** | `hari rata-rata stok bertahan (nilai stok / COGS harian)` | ≤ 40 hari |

## Push stok
Kandidat dealer untuk stok menua (> `policy.aging_days` 90 hari):
`product_mix cocok (kategori stok ∈ kategori yang pernah dibeli ATAU kategori kosong yang relevan dengan segmen) ∧ (due_in ≤ 7 ∨ lewat jadwal) ∧ sisa_limit ∉ {over limit, overdue}`. Harga bundle tidak pernah di bawah `policy.floor_margin` (9%). Kandidat diurutkan: jadwal order terdekat dulu, lalu omzet/bln.

## Rekomendasi order
Isi SO yang disarankan saat follow-up: 3 produk teratas dari komposisi order dealer (6 bulan) + 1 "pemanis" dari stok menua yang cocok + peringatan stok kritis cabang. Tidak pernah menyebut harga di bawah tier dealer.

## Follow-up (kebijakan)
Maksimal 1 follow-up per dealer per `policy.followup_gap` (14 hari); H-1 sebelum jadwal order; selalu membawa rekomendasi order, bukan "ada kebutuhan?". Follow-up ke dealer At risk/Churn dan follow-up ke-2 butuh approve.

## Tabel kasus uji (wajib di `internal/metrics/*_test.go`)
| Kasus | Input | Harapan |
|---|---|---|
| Key account | rhythm 14, last 6, sow 72, on_time 93 | cyc 0.43, status Key account, aktivitas normal, due_in 8 |
| At risk | rhythm 21, last 30 | cyc 1.43, status At risk, aktivitas menurun, lewat 9 hari |
| Churn | rhythm 28, last 70 | cyc 2.5, status Churn, berhenti |
| Baru | rhythm null, last 9 | status Baru, aktivitas baru, segmen Baru |
| Segmen A | rhythm 14, avg 62 jt | sering 2.14, Segmen A |
| Segmen B | rhythm 12, avg 7 jt | sering 2.5, Segmen B |
| Segmen C | rhythm 21, avg 38 jt | sering 1.43, Segmen C |
| Segmen D | rhythm 35, avg 12 jt | sering 0.86, Segmen D |
| Sisa limit tipis | limit 250, exposure 176, no late, pay 28 | room 0.296 → tipis (room < 0.40) |
| Sisa limit aman | limit 150, exposure 40, no late, pay 26 | room 0.73 → aman |
| Sisa limit over | limit 150, exposure 162 | over limit |
| Sisa limit overdue | limit 250, exposure 220, 1 invoice late | overdue |
| Cash | limit 0 | cash |
| Skor | rhythm 21, last 30, sow 40, mix 3/6, limit 150, exposure 162, on_time 58, pic 2 | r 49, p 40, k 50, n 0, i 80 → skor 44 |
| Push kandidat | stok LED 148 hari; dealer mix LED ✓, due_in 1, limit aman | kandidat |
| Push ditolak | dealer over limit | bukan kandidat |
