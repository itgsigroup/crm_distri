# Stage 10 — Odoo write-back (guarded)

## Tujuan
Tulis terbatas & terlacak ke Odoo: lead/opportunity baru dari ARC, activity untuk komitmen & tindakan, note chatter, usulan probabilitas.

## Baca dulu
`03` (Odoo write), `04` (human-only), CLAUDE.md §2.

## Kerjakan
1. `OdooWriter` terpisah: dry-run, audit, penanda "via ARC · sumber: …"; daftar putih: `crm.lead` create, `crm.lead.probability`, `mail.activity` create/done, `mail.message` note, `res.partner` create/update kontak.
2. Executor: `create_in_odoo` (dari tahap 08), komitmen `kami` → activity (done saat terpenuhi), `send_wa/email` → note di chatter, `write_probability` (selisih ≥ 15, approve).
3. Konflik `write_date` → batalkan + Signal `sync_conflict`. Stage tidak pernah ditulis (test).
4. UI: jumlah write & konflik di bar sinkron; tombol "Tulis N% ke Odoo".

## Acceptance criteria
- FakeOdoo: approve `write_probability` → payload benar + note evidence; konflik → tidak ada write + Signal.
- Komitmen baru → activity; selesai → done. Tidak ada `stage_id`/`unlink` di writer (grep test).
