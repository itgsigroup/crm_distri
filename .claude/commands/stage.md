Perintah eksekusi bertahap ARC. Argumen: `$ARGUMENTS` (salah satu: `next`, `status`, `plan`, `N`, `redo N`).

Lakukan persis protokol di `CLAUDE.md` §1:
1. Baca `.arc/progress.json` dan tentukan tahap yang relevan.
2. Untuk `status`: tampilkan tabel (id, nama, status, finished_at, ringkasan 1 baris) lalu saran tahap berikutnya. Berhenti.
3. Untuk `plan`: baca prompt tahap berikutnya, tampilkan rencana ≤ 12 baris (file, urutan, risiko, pertanyaan terbuka). Jangan mengubah kode. Berhenti.
4. Untuk `next` / `N` / `redo N`: tandai `status: "in_progress"` di progress.json, baca prompt tahap + knowledge yang dirujuk, kerjakan sampai acceptance criteria lolos (jalankan `make test`, `make lint`, `make dev` sendiri), perbarui progress.json + CHANGELOG.md, commit `feat(stage-NN): ...`, lalu laporkan ≤ 10 baris.
5. Jangan pernah menandai `done` bila ada acceptance criteria yang gagal. Bila terhalang kredensial, gunakan mock sesuai §1.5 dan tandai `done-with-mocks`.
