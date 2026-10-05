# 04 · Orchestrator — spesifikasi

Orchestrator adalah **satu-satunya** pintu masuk analisis. Signature:
```go
type Scope struct{ Kind string; ID string }        // all | screen | dealer | agent
type Trigger struct{ Source string; By string; Via string } // schedule|manual|mcp · user/client id · api|mcp
func (o *Orchestrator) Run(ctx context.Context, sc Scope, tr Trigger) (*domain.Cycle, error)
```
Dipanggil oleh: job `cycle.run` (tiap jam 06:00–20:00 WIB, `unique_opts` per jam), `POST /api/cycles` (tombol *Analisis ulang* dengan scope), dan tool MCP `orchestrator.run / reanalyze / agent.run`. Hanya **satu** siklus berjalan pada satu waktu (advisory lock `pg_advisory_xact_lock(42)`); permintaan lain mengantre atau ditolak dengan `409 cycle_running` (UI menampilkan "Orchestrator sedang berjalan").

## Enam tahap (semua dicatat di `cycle_stages`, dipancarkan ke SSE)
### 1. Ingest
Kumpulkan sinyal `processed_at is null` (atau semua sinyal 24 jam terakhir untuk scope dealer), hitung ulang `metrics` untuk dealer yang tersentuh, muat `policies` versi terbaru, muat `calibration_events` aktif (supresi). Output: `Input` per agen (lihat `05-agents.md`), masing-masing hanya berisi dealer dalam scope.

### 2. Analisis
Jalankan agen **paralel** (errgroup, maks 4 bersamaan). Scope `agent:<name>` → hanya agen itu. Setiap agen mengembalikan `[]Proposal` (belum tersimpan). Agen gagal/timeout → `agent_runs.status='failed'`, tahap `partial`, lanjut.

### 3. Sintesis & konflik
Kelompokkan proposal per dealer. Terapkan aturan berurutan; setiap aturan yang menyala menulis `conflicts` dan mengubah proposal (menunda, menggabungkan, atau menandai `suppressed`):
| Aturan | Kondisi | Resolusi |
|---|---|---|
| `credit_over_stock` | ada `push_stock`/`followup` DAN sisa limit dealer `over limit`/`overdue` | push/followup ditunda (`steps` ditambah "setelah pembayaran masuk"); proposal `collect` dari AI Penagihan dibuat bila belum ada |
| `collect_before_followup` | ada `followup` (jadwal order ≤ 7 hari) DAN `collect` untuk dealer sama | urutan: collect dulu; followup dijadwalkan otomatis setelah `payment` masuk (plan item `waiting`) |
| `margin_floor` | `price_counter`/`so_draft` dengan margin < `policy.margin.floor` | tidak pernah `auto`; eskalasi `approve` dengan counter yang masih di atas floor |
| `one_owner` | 2 sales aktif pada 1 dealer (interaksi ≥ 10/bln keduanya) | buat catatan memori: pemilik harga = interaksi terbanyak; proposal dari sales kedua diberi `prep` "koordinasi dengan <pemilik>" |
| `dedupe` | 2 proposal kind sama untuk dealer sama dari agen berbeda | gabung ke confidence tertinggi, `signal_ids` disatukan |
| `suppression` | ada `calibration_events.suppress_until ≥ today` untuk (agent, dealer, kind) | proposal `suppressed`, tidak masuk antrean |
| `followup_gap` | followup < 14 hari sejak follow-up terakhir ke dealer itu | `suppressed` dengan alasan |

### 4. Keputusan (otonomi)
Untuk setiap proposal, evaluasi `policy.autonomy.matrix[agent][kind]` + guard: `auto` hanya bila `confidence ≥ 0.8`, dealer bukan At risk/Churn, sisa limit `aman`, dan tidak ada konflik yang menyentuhnya. Hasil: `autonomy='auto'` → status `approved` dengan `decided_by = system` (tercatat), lainnya `proposed` → antrean Keputusan. Susun **Rencana hari ini** (`plan_items`) dari proposal: urut jam (auto lebih pagi, approve setelah jam kerja mulai), maksimal 10 item, sisanya tetap di antrean.

### 5. Eksekusi
Proposal `approved` (auto atau manusia) → `outbox` per kanal: `wa` (draft pesan dari `preview`, dikirim atas nama nomor sales pemilik dealer, **hanya** lewat `wa.Transport.Send`), `odoo_so_draft` (AI Order), `odoo_note`. Job `outbox.send` dengan retry; hasil kembali sebagai sinyal `kind='manual'` (jejak kirim). Tahap ini **tidak pernah** mengeksekusi proposal berstatus selain `approved/edited`.

### 6. Belajar
Baca keputusan manusia sejak siklus terakhir: penolakan dengan alasan → `calibration_events` (supresi 14 hari untuk (agent, dealer, kind); alasan "Tidak sesuai kebijakan" → tandai untuk review policy); `edited` → simpan diff sebagai contoh few-shot agen tersebut (maks 20 terbaru per agen); perbarui `agent_state.confidence = % proposal 30 hari yang approved/edited`. Tulis `cycles.note` (1 kalimat, oleh LLM dari ringkasan tahap, atau template bila LLM mati).

## Scope semantics
| scope | Ingest | Agen | Rencana |
|---|---|---|---|
| `all` | semua sinyal baru | semua | disusun ulang |
| `screen:orbit` / `segmen` | semua dealer aktif | followup, credit | tidak diubah; proposal baru masuk antrean |
| `screen:stock` | stok + dealer kandidat | stock | idem |
| `screen:credit` | invoice/pembayaran | credit, collect | idem |
| `dealer:<id>` | 24 jam sinyal dealer itu | semua, dibatasi dealer | item dealer itu diperbarui |
| `agent:<name>` | sesuai agen | satu agen | idem |

## Jalur LLM (`policy.llm.routing`)
`api` → provider langsung (Anthropic default, OpenAI cadangan bila error 2×). `mcp` → Orchestrator **tidak** memanggil LLM; ia menyiapkan `Input` dan menunggu klien MCP memanggil `analisis.*`/`orchestrator.agent.run` yang mengembalikan proposal lewat `orchestrator.submit` (lihat 06). `both` (default) → `api`, dan klien MCP boleh menambah. Jalur dicatat di `cycles.via`.

## Keluaran untuk UI
- `GET /api/cycles/latest` → status, stage saat ini, counters (signals, auto, decisions, conflicts), durasi, via.
- SSE `cycle_stage` tiap transisi (≤ 6 event per siklus) + `cycle_done`.
- `GET /api/cycles?limit=20` → riwayat; `GET /api/cycles/{id}` → stages, agent_runs, conflicts, proposals.

## Uji wajib (`internal/orchestrator`)
- Siklus dengan `llm.Fake` menghasilkan 6 stage `done`, counters benar, idempoten bila dijalankan dua kali pada jam sama.
- `credit_over_stock`: dealer over limit + push_stock → push ditunda + collect dibuat + 1 conflict.
- `collect_before_followup`: urutan plan item benar (collect sebelum followup, followup `waiting`).
- `margin_floor`: proposal margin 8,3% tidak pernah auto.
- `suppression`: penolakan kemarin → proposal serupa hari ini `suppressed`.
- Otonomi: proposal confidence 0.75 tidak auto meski matriks mengizinkan.
- Advisory lock: dua `Run` bersamaan → satu berjalan, satu `409`.
