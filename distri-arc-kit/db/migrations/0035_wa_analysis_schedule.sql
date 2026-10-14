-- +goose Up
-- Analisa WhatsApp of Pusat kendali by Claude through MCP every 2 hours (ADR 0037, Sam 14 Oct). A schedule with a key
-- is one the app knows: wa_analysis must end with Claude's orchestrator_submit for "Analisa WhatsApp", or the run fails
-- (Pusat kendali shows the label, MCP Claude keeps the history).
alter table mcp_schedules add column key text unique;

insert into mcp_schedules (key, name, prompt, cron, scopes, max_steps) values ('wa_analysis',
  'Analisa WhatsApp · tiap 2 jam',
  'Tugas: Analisa WhatsApp untuk Pusat kendali. Langkah:
1) Panggil orchestrator_reanalyze dengan scope "screen:wa" dan catat cycle_id dari hasilnya.
2) Bila status siklus masih "queued" atau "running", panggil orchestrator_status (paling banyak 3 kali) sampai siklus lewat tahap Ingest.
3) Panggil orchestrator_input_get dengan cycle_id itu dan agent "Analisa WhatsApp".
4) Ikuti instruksi di input: tulis overview untuk CEO, satu baris per percakapan (threads), dan catatan per pesan pelanggan yang membawa id (messages). Pakai angka dari totals persis.
5) Kirim lewat orchestrator_submit dengan cycle_id yang sama, agent "Analisa WhatsApp", overview, threads, dan messages.
Jangan menulis draft balasan dan jangan mengirim apa pun ke pelanggan. Laporan akhir 2–3 kalimat: nomor siklus, jumlah percakapan yang dibaca, berapa yang menunggu balasan, dan hal terpenting.',
  '0 */2 * * *', '{read,orchestrate}', 10)
on conflict (key) do nothing;

-- +goose Down
delete from mcp_schedules where key = 'wa_analysis';
alter table mcp_schedules drop column key;
