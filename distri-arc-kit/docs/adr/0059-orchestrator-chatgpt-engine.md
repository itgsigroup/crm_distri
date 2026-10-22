# ADR 0059 — Orchestrator: pilihan mesin MCP Claude atau ChatGPT (API)

Status: diterima · 2026-10-22 · Sam · mencabut sebagian ADR 0048 (tanpa ChatGPT) untuk Analisis ulang; menghapus
pilihan "Sistem" (ADR 0046) dari UI

## Konteks
Sam: "pada Orchestrator pilihan AI-nya jangan MCP Claude dan Sistem tanpa AI, harusnya MCP Claude dan ChatGPT."
Ditanya cara menjalankan ChatGPT (API otomatis di server, atau MCP di chatgpt.com yang harus dibuka orang), Sam memilih
**ChatGPT API**: jalan sendiri seperti routine Claude, dengan kunci OpenAI yang sudah dipakai AI Trigger (ADR 0051).

## Keputusan
- Pilihan mesin Analisis ulang di dock Orchestrator, halaman Orchestrator, dan Stasiun Orbit (Dashboard):
  **MCP Claude** (routine claude.ai / Claude Desktop, tanpa kunci API) atau **ChatGPT** (ChatGPT API di server).
  Masing-masing dengan modelnya (ChatGPT: GPT-5 mini bawaan, GPT-5, GPT-4.1, GPT-4.1 mini, o4-mini) dan effort.
  "Sistem" tidak lagi ditawarkan; pilihan lama "system" terbaca sebagai MCP Claude.
- `POST /cycles {via: "chatgpt", model}`: tanpa kunci ChatGPT atau dengan anggaran harian habis → 409 `no_api` dengan
  pesannya; selain itu siklus antre, `engine_model` = model ChatGPT (bukan model ChatGPT → `gpt-5-mini`), dan
  `mcp.engine = chatgpt_api`. Worker (`CycleMCPWorker`) menjalankan analisanya dengan ChatGPT bila `engine_model`
  adalah model ChatGPT (`Runner.Provider = openai`), memakai `LoadOpenAIKeyChosen`.
- Tetap dari ADR 0048: ChatGPT **tidak pernah mengambil alih sendiri** — `LoadOpenAIKey` (opsi kedua API, Analisa
  WhatsApp otomatis, API cadangan siklus per jam) tetap kosong; `TestChatGPTNeverUsed` tetap menjaganya. ChatGPT hanya
  menganalisa bila seseorang memilihnya untuk Analisis ulang itu.
- `via: "system"` tetap diterima API (MCP / jadwal), hanya tidak ada tombolnya.

## Konsekuensi
- Biaya ChatGPT per analisa tercatat di API & model AI (llm_calls / riwayat run) dan dibatasi anggaran harian yang sama.
- Satu AI per siklus: siklus ChatGPT tidak diambil alih Claude, dan sebaliknya.
