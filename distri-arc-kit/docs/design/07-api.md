# 07 · REST API & SSE

Base `/api`. JSON `snake_case`. Auth: cookie sesi (JWT HttpOnly, 12 jam) dari `POST /auth/login`; role `ceo|sales|admin|finance|warehouse`. Error: `{"error":{"code":"...","message":"...","details":{}}}` dengan HTTP status semantik. Pagination: `?limit=&cursor=` → `{items, next_cursor}`. Semua waktu RFC 3339 UTC; UI mengonversi ke WIB.

## Auth & pengguna
- `POST /auth/login {email,password}` · `POST /auth/logout` · `GET /me`
- `GET /users` (admin) · `POST /users` · `PATCH /users/{id}`

## Pusat kendali
- `GET /cycles/latest` → `{cycle, stages[], counters}`
- `GET /plan/today` → `plan_items[]` (dengan proposal tersemat)
- `GET /proposals?status=proposed&agent=&dealer_id=` · `GET /proposals/{id}`
- `POST /proposals/{id}/decide {decision:'approve'|'edit'|'reject', reason?, reason_text?, edited_payload?}` — **hanya role manusia**; `approve` → outbox; `reject` wajib `reason` (enum: tidak_tepat_waktu · salah_dealer · sudah_dilakukan · tidak_sesuai_kebijakan · konteks_kurang) → calibration.
- `GET /brief/today` → ringkasan Orchestrator (4 poin + provenance)
- `GET /dealers/due?days=7` · `GET /dealers/drift` · `GET /dealers/credit-tight`
- `GET /kpi?branch=` → order tepat jadwal, DSO, perputaran stok + target
- `GET /dashboard/trend?mode=months|days&month=YYYY-MM` → riwayat kartu Dashboard: 12 titik bulanan atau satu titik per hari (hari yang belum tiba `null`), dihitung ulang dengan rumus `metrics` pada riwayat yang dipotong di tanggal itu; seri kredit hanya untuk halaman Kredit; stok tidak punya riwayat (`unavailable`)
- `GET /agenda?date=` → per sales: due, lewat jadwal, tagih dulu
- `GET /stock/push` → kandidat push (ringkas)

## Orchestrator
- `POST /cycles {scope:'all'|'screen:orbit'|'dealer:<id>'|'agent:<name>', via?:'api'|'mcp'}` → `202 {cycle_id}` atau `409 cycle_running`
- `GET /cycles?limit=` · `GET /cycles/{id}` → stages, agent_runs, conflicts, proposals
- `GET /conflicts?cycle_id=` · `GET /agents` (state, confidence, last output) · `POST /agents/{name}/run {dealer_id?}`
- `GET /policies/autonomy` · `PUT /policies/autonomy` (ceo)
- `GET /mcp/clients` · `POST /mcp/clients` (admin; kembalikan token sekali) · `DELETE /mcp/clients/{id}` · `GET /mcp/calls?limit=`
- `GET /policies/mcp` · `PUT /policies/mcp` (ceo; `allow_send` ditolak)
- `GET /policies/llm` · `PUT /policies/llm {routing:'api'|'mcp'|'both', provider, model, fallback}`

## Chat
- `GET /chat/threads?tab=all|dealer|group_internal|new&q=` · `GET /chat/threads/{id}?before=` (30 pesan)
- `POST /chat/threads/{id}/messages {body}` → membuat **proposal `kind='reply'` auto-approved oleh pengirim manusia** (tercatat `decided_by`), masuk outbox → kirim. (Manusia mengirim sendiri = keputusan.)
- `POST /chat/threads/{id}/messages {body, reply_to?}` — `reply_to` = id bubble yang dibalas (WhatsApp "Balas"; harus sudah punya id WhatsApp, kalau belum 409). Bubble membawa `quote {id, direction, from_name, body, media_kind}` dan `can_reply`.
- `PATCH /chat/threads/{id}/messages/{mid} {body}` → edit pesan teks dari nomor itu (ADR 0029): masih di antrean → teks outbox diubah (200); sudah terkirim ≤ 15 menit → proposal `reply` baru + outbox `kind=edit` (202); selain itu 409.
- `GET /screen-analysis/{stock|credit}` → Analisa Stok / Analisa Kas terakhir yang ditulis AI (ADR 0030): `{analysis: {cycle_number, generated_at, source, overview, lines: {<id baris>: {note, confidence}}} | null}`.
- `GET /chat/threads/{id}/messages/{mid}/media` → file media pesan (foto, video, VN, dokumen) diambil dari WhatsApp saat dibuka, tidak disimpan (ADR 0029); 404 tanpa referensi, 410 hilang di WhatsApp, 503 nomor belum terhubung, 409 bukan transport Baileys. Bubble membawa `media {kind, mime, file_name, size, seconds, ptt, viewable}`, `edited_at`, `can_edit`.
- `GET /chat/threads/{id}/context` → dealer, langkah berikutnya, ekstraksi (SO terdeteksi, komitmen), produk favorit
- `POST /chat/identify {wa_number}` (AI Prospek) · `POST /identifications/import` (CSV Getcontact manual)
- `GET /internal-numbers` · `POST /internal-numbers` · `DELETE /internal-numbers/{wa}` · `GET /wa/groups` · `PATCH /wa/groups/{id} {kind, read_enabled}`
- `GET /wa/status` · `POST /wa/pair` (QR, whatsmeow) · `POST /wa/cloud/webhook` (Cloud API)

## Orbit · Segmen · Peta relasi
- `GET /orbit?sales=` → dealer dengan `cyc, status, sow, credit_state, score, segment, due_in, rhythm, last`
- `GET /orbit/summary` → per status: jumlah, omzet/bln · `GET /orbit/movers`
- `GET /segmen?sales=` → titik (freq, avg_order, prev 3 bulan) · `GET /segmen/summary` · `GET /segmen/movers`
- `GET /relasi?period=30|60|90|180&sales=` → nodes (sales, dealer), edges (interaksi) · `GET /relasi/insights`

## Dealer
- `GET /dealers?q=&status=&segment=&sales=` (daftar) · `GET /dealers/{id}` (header + metrics + 5 komponen)
- `GET /dealers/{id}/next` (proposal aktif) · `/orders?months=6` · `/mix` · `/credit` · `/memo` · `/contacts` · `/commitments` · `/timeline?limit=`
- `POST /dealers/{id}/sow {sow, note}` (sales konfirmasi) · `POST /dealers/{id}/reanalyze` (= `POST /cycles scope dealer`)

## Push stok · Kredit & kas
- `GET /stock/aging?branch=` · `GET /stock/critical` · `GET /stock/sales-by-product?days=30`
- `GET /credit/overview` · `GET /credit/dealers` · `GET /credit/exposure` · `GET /credit/forecast?days=30`

## Pengaturan
- `GET /policies` · `PUT /policies/{key}` (ceo; versi naik, history) · `GET /policies/{key}/history`
- `GET /connections` (odoo, wa, identifikasi, llm, mcp: status) · `POST /connections/odoo/test`
- `GET /calibration` (confidence per agen + 10 pelajaran terakhir)

## SSE `GET /events`
Event: `cycle_stage {cycle_id, stage, status}`, `cycle_done {cycle_id, counters}`, `proposal_changed {id, status}`, `chat_message {thread_id}`, `wa_status {state}`. Heartbeat 25 dtk. Frontend: satu koneksi, invalidasi query per event.

## Keamanan
- CSRF: cookie `SameSite=Strict` + header `X-Requested-With`.
- RBAC: `sales` hanya melihat dealer miliknya di daftar (filter server), tetap bisa membuka dealer lain lewat Chat grup; `finance` boleh `decide` untuk `collect/credit_*`; `ceo` semua.
- Audit: semua `PUT/POST/DELETE` ke `audit_log`.
