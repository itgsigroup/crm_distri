# 01 — Produk ARC (baca selalu)

## Tesis
CRM konvensional = database yang diberi makan manusia → datanya telat dan tidak lengkap. ARC membalik: **AI mengisi database, manusia memutuskan**. Database adalah by-product dari penalaran.

## Enam prinsip desain
1. Zero data entry: capture → infer → confirm. 2. System of judgment (apa langkah berikutnya, kenapa). 3. Evidence over opinion (stage/probabilitas dari bukti). 4. Memory, bukan record (memori akun hidup). 5. Bounded autonomy (agen bertindak dalam policy; manusia meng-approve tindakan berisiko). 6. Interface = brief + percakapan.

## Enam lapis arsitektur
Signal Layer (ingest) → Relationship Graph (temporal, provenance) → Reasoning Core (event-driven + terjadwal) → Agent Layer → Human Layer → Trust Layer (policy, explainability, audit, privasi).

## Agen (lihat 06-agents.md)
Capture · Research · Follow-up · Meeting prep · Forecast · Hygiene · Collection · Identity · (Pricing sebagai sub-agen policy).

## Permukaan (UI) — lihat 05-ui-spec.md
Hari ini (brief + keputusan) · Chat (WhatsApp) · Relasi (akun + peta 3D) · Penjualan (pipeline Odoo + medan + prospek & funnel + tender + tim) · Kas (Won→Lunas, aging, prediksi kas) · Pengaturan (sumber, WhatsApp, nomor internal, AI & model, MCP & API). Satu kotak "Tanya ARC" (⌘K) di semua layar.

## Model tindakan (Action)
Semua saran agen adalah `Action` dengan siklus: `proposed → (approved | edited | rejected | snoozed) → executed | cancelled`. Setiap Action: agen pengusul, akun/deal, tipe, judul, `why` (bukti, id interaksi), `prep` (apa yang disiapkan), preview (draft), langkah setelah disetujui, confidence, model. Penolakan wajib punya alasan dari daftar (`tidak tepat waktu`, `salah kontak/jalur`, `sudah dilakukan`, `tidak sesuai kebijakan`, `konteks agen kurang`) + teks bebas → masuk kalibrasi: agen tidak mengusulkan tindakan serupa untuk akun itu selama 14 hari kecuali sinyal berubah; tingkat persetujuan per agen dipantau, < 60% → kalibrasi ulang.

## Kadens (keputusan arsitektur: batch, bukan event-driven)
Capture/hygiene/follow-up/meeting-prep: **per jam**. Forecast, health, brief: **harian** (brief 06.45 & 16.00). Ask: on-demand. Webhook hanya untuk WhatsApp inbound dan form web. Alasan: siklus deal berhari-hari, biaya & kesederhanaan.

## Metrik keberhasilan produk ("Denyut bisnis")
Lead→cash median, DSO, Won→invoice, waktu respons lead baru, pipeline coverage, akurasi commit; plus 3 metrik flywheel: referral rate, expansion rate, time-to-delight (Won→BAST).
