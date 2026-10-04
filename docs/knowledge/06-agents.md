# 06 — Agen: input, output, kadens, guardrail (baca di tahap 04–06, 09–11)

Format umum: setiap agen adalah fungsi murni `run(context) -> list[Proposal|Fact]` dengan akses baca ke graph dan tulis hanya ke tabel hasilnya sendiri + `Action` (status `proposed`). Semua output membawa provenance. Prompt LLM disimpan di `packages/core/prompts/<agent>/<version>.md` dan diuji dengan fixture.

| Agen | Kadens | Tier | Input | Output | Guardrail |
|---|---|---|---|---|---|
| **Capture** | per jam | light | Interaction baru (email, WA, meeting notes, dokumen) | identity resolution (person↔account), Commitment (kami/mereka), Signal, sentimen, ringkasan interaksi | idempoten per `raw_ref`; tidak membuat Person baru tanpa ≥ 2 sinyal identitas; PII masked |
| **Hygiene** | per jam | light | Person/Account | gabung duplikat (usulan), lengkapi field, tandai internal | merge hanya sebagai Action bila confidence < 0.9 |
| **Research** | per jam (lead baru), harian | heavy | InboundContact, Tender, Account baru | overview orang/perusahaan, tangga solusi, pertanyaan pain point, match tender, ruang ekspansi | hanya sumber publik + data internal; tidak menebak PII |
| **Follow-up** | per jam | heavy | Commitment jatuh tempo, Signal silent/competitor, Opportunity | Action draft email/WA dalam gaya pemilik kanal, re-engagement | tidak mengirim; hormati `gov_reminder_min_days`; 1 draft aktif per akun |
| **Meeting prep** | per jam (H-1) | heavy | Calendar event + akun | brief 1 halaman + poin pembuka (mis. PO tertahan) | tidak untuk meeting internal < 30 mnt (kalibrasi) |
| **Forecast** | harian | heavy | Opportunity + health + histori | commit/best/pipeline, arc_probability, skenario, weighted vs sales | commit = Won + verbal berbukti tertulis & health ≥ 80 |
| **Collection** | harian | light/heavy | L2C, invoice, pola bayar | Action: buat invoice (BAST clear), pengingat bernada sesuai pola bayar, tanya dokumen SPM; prediksi kas 30 hari | pemerintah: tidak mengingatkan < 30 hari; nada ramah untuk pelanggan tepat waktu |
| **Identity** | on-event (inbound) | light+heavy | nomor/kontak baru | identifikasi multi-sumber, fit score, status | hanya inbound; retensi 90 hari |
| **Chief/Brief** | 06.45 & 16.00 | heavy | semua di atas | Brief 4 poin (gap, risiko, orang, peluang) + Keputusan | setiap poin punya evidence; ≤ 4 poin |

## Struktur prompt (semua agen)
1. Peran & tujuan singkat. 2. Skema output JSON (Pydantic) — wajib. 3. Konteks: akun, 5–20 interaksi relevan (masked), policy. 4. Aturan: kutip bukti verbatim (≤ 200 karakter) untuk setiap klaim; confidence; bila bukti tidak cukup → kosongkan, jangan mengarang. 5. Contoh 1–2 (few-shot) dari fixture GSI. Versi prompt di header; perubahan prompt = versi baru + regresi test.

## Kalibrasi
Simpan setiap keputusan manusia atas Action. Hitung acceptance rate per agen & tipe per 30 hari. Penolakan dengan alasan `tidak sesuai kebijakan` 3× pada pola yang sama → buat aturan larangan otomatis (ditampilkan di "Yang dipelajari") + notifikasi CEO.
