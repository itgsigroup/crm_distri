# Stage 09 — Gmail & Calendar capture

## Tujuan
Email dan kalender masuk sebagai Interaction (kanal kedua setelah WA), identity resolution email/domain, Meeting prep dari kalender nyata, draft balasan sebagai Gmail draft.

## Baca dulu
`03` (Gmail/Calendar), `02` (Interaction, Person), `04` (consent, PII).

## Kerjakan
1. OAuth per pengguna, token terenkripsi, consent tercatat.
2. `capture_gmail` per jam: incremental (historyId), filter thread yang cocok Person/Account (domain) atau dibalas pengguna; body bersih (kutipan & signature dibuang), lampiran meta (unduh PDF/DOCX penawaran/proposal/PO/BAST).
3. `capture_calendar`: H-7..H+14 → Interaction(meeting) + akun via peserta; Meeting prep agent memakai ini.
4. Identity resolution email: alamat persis → Person; domain → Account; Person baru hanya ≥ 2 sinyal; sisanya `Unresolved` untuk Hygiene.
5. Executor: Action `send_email` → **Gmail draft** di mailbox pemilik (tidak pernah send).
6. `FakeMailSource` dari ≥ 12 `.eml` realistis; ekstraksi tahap 03 berjalan untuk email.

## Acceptance criteria
- 12 email fixture → 12 Interaction, 0 duplikat pada run kedua, 9 terhubung akun, 2 Unresolved, 1 diabaikan; body tanpa kutipan.
- 3 meeting kalender → Meeting prep Action H-1 terbentuk.
- Tanpa kredensial → `done-with-mocks`.
