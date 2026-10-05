Perintah eksekusi bertahap Distri ARC Orbit. Argumen: `$ARGUMENTS` (salah satu: `next`, `status`, `plan`, `N`, `redo N`).

Lakukan persis protokol di `CLAUDE.md` §1:
1. Baca `.arc/progress.json` dan tentukan tahap yang relevan.
2. `status`: tampilkan tabel (id, nama, status, finished_at, ringkasan 1 baris) lalu saran tahap berikutnya. Berhenti.
3. `plan`: baca prompt tahap berikutnya + design doc yang dirujuk, tampilkan rencana ≤ 12 baris (file, urutan, risiko, pertanyaan terbuka). Jangan mengubah kode. Berhenti.
4. `next` / `N` / `redo N`: tandai `status: "in_progress"`, baca prompt tahap + design doc yang dirujuk + ADR, kerjakan sampai semua acceptance criteria lolos (jalankan `make check` dan `make dev` sendiri), perbarui `.arc/progress.json` + `CHANGELOG.md` + README bagian tahap, commit `feat(stage-NN): ...`, laporkan ≤ 10 baris.
5. Jangan pernah menandai `done` bila ada acceptance criteria yang gagal. Bila terhalang kredensial eksternal, gunakan fake sesuai §1.5 dan tandai `done-with-fakes`.
