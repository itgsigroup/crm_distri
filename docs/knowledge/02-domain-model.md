# 02 — Model domain (baca selalu)

Semua tabel punya: `id`, `created_at`, `updated_at`, dan bila berasal dari sistem luar: `source_system`, `source_id`, `source_write_date`. Semua fakta hasil penalaran punya **provenance**: `evidence` (daftar `{interaction_id|document_id, quote, at}`), `confidence` (0–1), `model`, `prompt_version`.

## Entitas inti
- **Account** — perusahaan/instansi. Field: name, sector (pemerintah/enterprise/pendidikan/...), branch (cabang GSI), owner_user, odoo_partner_id, odoo_company_id, health (0–100), health_trend_30d, memory (teks memori akun, versi), tags. Relasi: people, opportunities, commitments, interactions, installed_systems, whitespace.
- **Person** — kontak. Field: name, role/title, account_id, phones[], emails[], wa_ids[], stakeholder_tag (`decision|champion|influencer|user|procurement|ghost|former`), strength (0–3, dari intensitas interaksi), is_internal (bool), internal_unit, last_contact_at.
- **Opportunity** — cermin `crm.lead` Odoo + lapisan ARC. Field Odoo (read-only di ARC): name, expected_revenue, probability (manual sales), stage (Odoo: Baru/Berkualifikasi/Penawaran/Won/Lost — nama diambil dari Odoo saat sync, jangan hardcode), date_deadline, tags, priority, activity_state, user_id. Field ARC: health, health_breakdown {engagement, multithreading, momentum, fit, sentiment}, signal (`negosiasi|verbal_commit|kontrak|...`), stage_evidence, risk_flags[], arc_probability, next_action_id.
- **Interaction** — satu kejadian: email, wa_message, wa_group_message, meeting, call, document, form, erp_event, note. Field: channel, direction (in/out), occurred_at, participants (person_ids, user_ids), thread_id, subject, body_text (dimasking bila perlu), attachments[], raw_ref (message-id / wa id / odoo id), account_id?, opportunity_id?, group_id?, extracted (bool), extraction_version.
- **Commitment** — janji dua arah. Field: who (`kami|mereka`), text, due_at, status (`open|done|late|cancelled`), origin_interaction_id, account_id, opportunity_id?, owner_user, resolved_by_interaction_id?, escalation_level.
- **Signal** — sinyal terdeteksi: type (`competitor_mentioned|champion_moved|silent|single_threaded|payment_on_time|payment_late|po_overdue|stock_risk|...`), severity, account_id, opportunity_id?, evidence, detected_at, resolved_at, acknowledged_by.
- **Action** — lihat 01. Field: agent, type, title, account_id, opportunity_id?, due, why, prep, preview, steps[], confidence, model, status, decision {user, at, reason, note}, executed_at, execution_log.
- **Policy** — aturan dalam bahasa natural + parameter terstruktur (mis. `discount_max_without_ceo=5`, `credit_limit[account]`, `quiet_threshold_days=14`, `gov_reminder_min_days=30`). Versi & audit.
- **AuditLog** — siapa/agen apa melakukan apa, kapan, pada objek apa, dengan payload hash.
- **User** — pengguna internal; role (`ceo|manager|sales|finance|ops`), branch, odoo_user_id, wa_numbers[].
- **InternalNumber** — nomor internal: person/unit, phone, branch, source (`talenta|manual|arc_suggested`), confirmed_by.
- **ChatThread / ChatGroup** — thread WA per nomor sales; grup dengan type (`external|internal`), members (person/internal), read_policy.
- **InboundContact** (Prospek) — nomor/kontak baru: phone, first_message, via_user, identification {name, role, company, sources[{source, value, at}], confidence}, fit_score, status (`unknown|identified|qualified|lead|not_prospect`), solutions[], pain_questions[], odoo_lead_id?.
- **InstalledSystem** — sistem terpasang: account_id, system, installed_at, warranty_end, service_contract (bool/until).
- **CashItem / L2C** — cermin SO→project→BAST→invoice→payment: so_id, project_id, stage (`persiapan|pemasangan|bast|invoice|menunggu_bayar|lunas`), stage_entered_at, benchmark_days, invoice_id?, due_at?, paid_at?, blocked_reason.
- **Tender** — dari LPSE/e-katalog: title, agency, hps, deadline, match_score, reasons[], status.
- **LLMCall** — log: tier, provider, model, tokens_in/out, cost_est, purpose, input_hash, duration_ms, ok.

## Health score (v1, 0–100)
health = 0.25·engagement + 0.20·multithreading + 0.20·momentum + 0.20·fit + 0.15·sentiment.
- engagement: frekuensi interaksi 30 hari vs ritme normal akun (median jarak antar interaksi 90 hari terakhir).
- multithreading: jumlah kontak aktif (≥ 1 interaksi 30 hari) berbobot tag; 1 kontak → ≤ 20; ada decision-maker aktif → +30.
- momentum: tren 14 hari (naik/turun), ada komitmen mereka yang terpenuhi → +; komitmen mereka lewat → −.
- fit: kecocokan solusi (spek, anggaran teridentifikasi, referensi serupa).
- sentiment: dari ekstraksi LLM pada 5 interaksi terakhir.
Band: ≥ 70 sehat, 50–69 perhatian, < 50 berisiko. Semua komponen menyimpan evidence.

## Stage & sinyal
Stage **milik Odoo**. ARC hanya menyimpan `signal` (sub-status dari bukti) dan `stage_evidence`, serta boleh mengusulkan `arc_probability` (tulis-balik hanya di Stage 08 dengan aturan: selisih ≥ 15 poin → Action "tulis probabilitas ke Odoo", butuh approve).

## Aturan idempotensi
Kunci unik: email `message-id`; WA `wamid`; Odoo `(model, id)` + `write_date`; komitmen: hash(account_id, who, normalized_text, due_week). Ekstraksi ulang dengan `extraction_version` baru boleh memperbarui, tidak menggandakan.
