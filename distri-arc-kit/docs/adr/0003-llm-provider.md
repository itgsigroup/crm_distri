# ADR 0003 — LLM: provider abstraction, Anthropic default, OpenAI cadangan, Fake untuk uji
**Status**: diterima · 2026-10-05

## Keputusan
`internal/llm.Provider { Complete(ctx, Request) (Response, error) }` dengan `Request{Purpose, System, Messages, Schema, MaxTokens}`; keluaran selalu JSON tervalidasi. Implementasi `anthropic` (model dari `policy.llm`), `openai`, `fake`. Router `policy.llm.routing`: `api` | `mcp` | `both`. Batch per jam 06–20 WIB; biaya dicatat per panggilan.

## Konsekuensi
- Agen tidak pernah mengimpor SDK provider langsung.
- Prompt disimpan sebagai file `.md` berversi di `internal/llm/prompts/`, diuji dengan snapshot.
- PII masking wajib sebelum request eksternal.
