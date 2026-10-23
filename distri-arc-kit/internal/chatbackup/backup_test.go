package chatbackup_test

import (
	"archive/zip"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/chatbackup"
	"distri-arc/internal/clock"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// Cadangan chat (ADR 0061): one zip per month with a WhatsApp-format .txt per chat; a group is one chat for every
// GSI number in it; a complete month is never rewritten (the purge empties the database, the file stays); a chat
// marked private is removed from every month; old months rotate out.
func TestMonthlyBackup(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	wib := func(mo time.Month, d, h int) time.Time { return time.Date(2026, mo, d, h, 0, 0, 0, clock.WIB) }
	now := wib(10, 23, 2)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := st.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var sinar string
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where seed_key = 'seed:thread:c-sinar'").Scan(&sinar); err != nil {
		t.Fatal(err)
	}
	const jid = "6281100001111@s.whatsapp.net"
	exec(`update chat_threads set wa_jid = $2, title = 'Toko Sinar', phone_name = 'Mbak Rina' where id = $1`, sinar, jid)
	msg := func(thread, id, dir, from, body, status string, at time.Time) {
		exec(`insert into chat_messages (thread_id, wa_msg_id, direction, from_name, body, sent_at, status) values ($1, $2, $3, $4, $5, $6, $7)`,
			thread, id, dir, from, body, at, status)
		exec(`insert into chat_message_keys (wa_msg_id, message_id, sent_at) select $1, id, sent_at from chat_messages where wa_msg_id = $1`, id)
	}
	msg(sinar, "SEP-1", "in", "Rina", "order september", "received", wib(9, 10, 9))
	msg(sinar, "OCT-1", "in", "Rina", "order oktober", "received", wib(10, 20, 9))
	msg(sinar, "OCT-X", "out", "", "gagal terkirim", "failed", wib(10, 20, 10))
	// one group held by two GSI numbers: each message stored under the number that got it first
	var g1, g2 string
	for i, p := range []*string{&g1, &g2} {
		if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, account) values ('group', $1, 'Installer Semarang', $2) returning id`,
			"120363@g.us", []string{"628001", "628002"}[i]).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	msg(g1, "G-1", "in", "Budi", "siap kirim", "received", wib(10, 21, 9))
	msg(g2, "G-2", "out", "", "ok", "sent", wib(10, 21, 10))

	dir := t.TempDir()
	files, err := chatbackup.Run(ctx, st.Pool, dir, now, 12)
	if err != nil {
		t.Fatal(err)
	}
	months := map[string]chatbackup.File{}
	for _, f := range files {
		months[f.Month] = f
	}
	if _, ok := months["2026-09"]; !ok {
		t.Fatalf("september not written: %+v", files)
	}
	oct := read(t, filepath.Join(dir, "chat-2026-10.zip"))
	sinarTxt := find(t, oct, "Chat WhatsApp dengan Mbak Rina - Toko Sinar.txt")
	if !strings.Contains(sinarTxt, "20/10/26 09.00 - Mbak Rina - Toko Sinar: order oktober") || strings.Contains(sinarTxt, "gagal terkirim") || strings.Contains(sinarTxt, "order september") {
		t.Fatalf("october sinar:\n%s", sinarTxt)
	}
	group := find(t, oct, "Chat WhatsApp dengan Installer Semarang.txt")
	if !strings.Contains(group, "21/10/26 09.00 - Budi: siap kirim") || !strings.Contains(group, ": ok") {
		t.Fatalf("group:\n%s", group)
	}
	if n := count(oct, "Installer Semarang"); n != 1 {
		t.Fatalf("the group is %d entries, want 1", n)
	}

	// the purge empties september; a later run never rewrites a complete month
	exec(`delete from chat_messages where wa_msg_id = 'SEP-1'`)
	if _, err := chatbackup.Run(ctx, st.Pool, dir, now.Add(24*time.Hour), 12); err != nil {
		t.Fatal(err)
	}
	sep := read(t, filepath.Join(dir, "chat-2026-09.zip"))
	if !strings.Contains(find(t, sep, "Chat WhatsApp dengan Mbak Rina - Toko Sinar.txt"), "order september") {
		t.Fatal("september backup lost after the purge")
	}

	// "Obrolan pribadi": the chat leaves every month
	if n, err := chatbackup.Forget(dir, jid); err != nil || n < 2 {
		t.Fatalf("forget: %d %v", n, err)
	}
	list, err := chatbackup.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range list {
		if count(read(t, filepath.Join(dir, f.Name)), "Toko Sinar") != 0 {
			t.Fatalf("%s still holds the private chat", f.Name)
		}
		if f.Month == "2026-10" && (f.Chats == 0 || f.Messages == 0 || f.Size == 0) {
			t.Fatalf("october summary %+v", f)
		}
	}

	// rotation: keep 1 month → only october
	if _, err := chatbackup.Prune(dir, now, 1); err != nil {
		t.Fatal(err)
	}
	if list, _ := chatbackup.List(dir); len(list) != 1 || list[0].Month != "2026-10" {
		t.Fatalf("after prune: %+v", list)
	}
}

func read(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func find(t *testing.T, z map[string]string, suffix string) string {
	t.Helper()
	for name, body := range z {
		if strings.HasSuffix(name, "/"+suffix) {
			return body
		}
	}
	names := make([]string, 0, len(z))
	for n := range z {
		names = append(names, n)
	}
	t.Fatalf("no %q in %v", suffix, names)
	return ""
}

func count(z map[string]string, part string) int {
	n := 0
	for name := range z {
		if strings.Contains(name, part) {
			n++
		}
	}
	return n
}
