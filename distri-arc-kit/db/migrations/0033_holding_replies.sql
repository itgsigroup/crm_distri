-- +goose Up
-- Response time counts until the customer gets an answer (ADR 0034, Sam 14 Oct): a holding reply from our side —
-- "tunggu sebentar ya", "saya konfirmasi dulu", "nanti saya kabari", "sedang kami cek" — does not answer the customer
-- and does not stop the clock. One definition, here: a short message (≤ 160 characters) without any figure (a price,
-- a quantity, a date is content) and without media, saying only that an answer will come.
-- +goose StatementBegin
create function is_holding_reply(body text) returns bool language sql immutable parallel safe as $fn$
  select coalesce(btrim(body), '') <> '' and length(body) <= 160 and body !~ '[0-9]' and left(btrim(body), 1) <> '['
    and lower(body) ~ ('('
      -- tunggu sebentar / mohon ditunggu / ditunggu ya
      || 'mohon\s+(di)?tunggu|harap\s+(di)?tunggu|\m(di)?tunggu\s+(sebentar|sebntar|sbntr|bentar|bntr|ya+|y|dulu|dlu|nggih|ggih|kak|pak|bu)\M'
      -- sebentar ya / bentar ya
      || '|\m(sebentar|sebntar|sbntr|bentar|bntr)\s+(ya+|y|nggih|ggih|kak|pak|bu|mas|mbak|dulu|dlu)\M'
      -- saya cek dulu / konfirmasi dulu / tanyakan dulu / hitung dulu
      || '|\m(cek|cekin|cekkan|dicek|dicekkan|konfirmasi|konfirmasikan|dikonfirmasi|konfirm|tanya|tanyakan|ditanyakan|tanyain|cari|carikan|dicarikan|hitung|hitungkan|dihitung|hitungin|siapkan|buatkan|bikinkan)\s+(dulu|dlu|ya+|y|nggih|ggih|sebentar|bentar)\M'
      -- saya tanyakan ke gudang / cek ke admin
      || '|\m(cek|cekkan|konfirmasi|konfirmasikan|tanya|tanyakan|koordinasi|koordinasikan)\s+(ke|di|dengan|sama)\s+(gudang|kantor|atasan|admin|bos|pusat|tim|finance|keuangan|accounting|teknisi|supplier|vendor|principal|pimpinan|manager|manajer)\M'
      -- nanti saya kabari / segera kami infokan
      || '|\m(nanti|ntar|ntr|segera)\s+((saya|sy|aku|kami|kita)\s+)?(di)?(kabari|kabarin|kabarkan|infokan|info|informasikan|update|updatekan|kirim|kirimkan|balas|jawab|hubungi|telp|telpon)'
      -- sedang kami cek / lagi dicek / masih diproses
      || '|\m(sedang|lagi|masih)\s+((saya|sy|kami|kita)\s+)?(di)?(cek|cekkan|proses|tanyakan|konfirmasi|konfirmasikan|hitung|cari|carikan|koordinasikan)'
      -- izin cek / on process
      || '|\m(izin|ijin)\s+(cek|konfirmasi|tanya|tanyakan)\M|\mon\s*process\M'
      || ')')
$fn$;
-- +goose StatementEnd
alter table chat_messages add column holding bool generated always as (is_holding_reply(body)) stored;

-- a new monthly partition carries the generated column, and rows moved out of the default partition are copied
-- without it (a generated column cannot be written)
-- +goose StatementBegin
create or replace function arc_attach_month(tbl text, col text, m date) returns bool language plpgsql as $$
declare
  part text := format('%s_y%sm%s', tbl, to_char(m, 'YYYY'), to_char(m, 'MM'));
  lo timestamptz := m::timestamptz;
  hi timestamptz := (m + interval '1 month')::timestamptz;
  cols text;
begin
  if to_regclass(format('%I.%I', current_schema(), part)) is not null then
    return false;
  end if;
  execute format('create table %I (like %I including defaults including constraints including generated)', part, tbl);
  if to_regclass(format('%I.%I', current_schema(), tbl || '_default')) is not null then
    select string_agg(quote_ident(column_name), ', ' order by ordinal_position) into cols
    from information_schema.columns where table_schema = current_schema() and table_name = tbl and is_generated = 'NEVER';
    execute format('with moved as (delete from %I where %I >= $1 and %I < $2 returning *) insert into %I (%s) select %s from moved',
                   tbl || '_default', col, col, part, cols, cols) using lo, hi;
  end if;
  execute format('alter table %I attach partition %I for values from (%L) to (%L)', tbl, part, lo, hi);
  return true;
end $$;
-- +goose StatementEnd

alter table wa_message_insights drop constraint wa_message_insights_state_check;
alter table wa_message_insights add constraint wa_message_insights_state_check
  check (state in ('replied','waiting','needs_reply','closing','reply','follow_up','holding'));

-- +goose Down
-- +goose StatementBegin
create or replace function arc_attach_month(tbl text, col text, m date) returns bool language plpgsql as $$
declare
  part text := format('%s_y%sm%s', tbl, to_char(m, 'YYYY'), to_char(m, 'MM'));
  lo timestamptz := m::timestamptz;
  hi timestamptz := (m + interval '1 month')::timestamptz;
begin
  if to_regclass(format('%I.%I', current_schema(), part)) is not null then
    return false;
  end if;
  execute format('create table %I (like %I including defaults including constraints)', part, tbl);
  if to_regclass(format('%I.%I', current_schema(), tbl || '_default')) is not null then
    execute format('with moved as (delete from %I where %I >= $1 and %I < $2 returning *) insert into %I select * from moved',
                   tbl || '_default', col, col, part) using lo, hi;
  end if;
  execute format('alter table %I attach partition %I for values from (%L) to (%L)', tbl, part, lo, hi);
  return true;
end $$;
-- +goose StatementEnd
delete from wa_message_insights where state = 'holding';
alter table wa_message_insights drop constraint wa_message_insights_state_check;
alter table wa_message_insights add constraint wa_message_insights_state_check
  check (state in ('replied','waiting','needs_reply','closing','reply','follow_up'));
alter table chat_messages drop column holding;
drop function is_holding_reply(text);
