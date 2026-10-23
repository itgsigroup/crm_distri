package api_test

import (
	"context"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// Ekspor chat and Cadangan chat (ADR 0061): whoever reads a chat exports it as WhatsApp text; the company's monthly
// backups are listed, written and downloaded by the CEO/admin only.
func TestChatExportAndBackup(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	h := api.New(config.Config{Env: "dev", ChatBackupDir: dir, ChatBackupKeepMonths: 12}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	var thread string
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where seed_key = 'seed:thread:c-sinar'").Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status)
		values ($1, 'EXP-1', 'in', 'tolong kirim NVR 16ch', now() - interval '1 hour', 'received')`, thread); err != nil {
		t.Fatal(err)
	}
	fetch := func(method, path, user string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, nil)
		req.Header.Set("X-Dev-User", user)
		req.Header.Set("X-Requested-With", "distri-arc")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	// the sales holding the chat exports it
	res, body := fetch(http.MethodGet, "/api/chat/threads/"+thread+"/export", "andi@gsi.co.id")
	_, params, _ := mime.ParseMediaType(res.Header.Get("Content-Disposition"))
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(params["filename"], "Chat WhatsApp dengan ") || !strings.HasSuffix(params["filename"], ".txt") ||
		!strings.Contains(body, ": tolong kirim NVR 16ch\n") || !strings.Contains(body, "Salinan chat dari GSI Orbit") {
		t.Fatalf("export %d %q\n%s", res.StatusCode, res.Header.Get("Content-Disposition"), body)
	}
	var audited int
	_ = st.Pool.QueryRow(ctx, `select count(*) from audit_log where action = 'chat.export'`).Scan(&audited)
	if audited != 1 {
		t.Fatalf("export audited %d×", audited)
	}

	// a sales sees no company backup
	if res, _ := fetch(http.MethodGet, "/api/chat/backups", "andi@gsi.co.id"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("sales list: %d", res.StatusCode)
	}
	if res, _ := fetch(http.MethodPost, "/api/chat/backups", "andi@gsi.co.id"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("sales backup: %d", res.StatusCode)
	}
	// the CEO backs up now, lists and downloads
	if res, body := fetch(http.MethodPost, "/api/chat/backups", "sam@gsi.co.id"); res.StatusCode != http.StatusOK || !strings.Contains(body, `"written"`) {
		t.Fatalf("backup now: %d %s", res.StatusCode, body)
	}
	var list struct {
		Items []struct {
			Name     string `json:"name"`
			Messages int    `json:"messages"`
		} `json:"items"`
	}
	if code := getJSON(t, srv, "/api/chat/backups", "sam@gsi.co.id", &list); code != http.StatusOK || len(list.Items) == 0 || list.Items[0].Messages == 0 {
		t.Fatalf("list %d %+v", code, list)
	}
	res, body = fetch(http.MethodGet, "/api/chat/backups/"+list.Items[0].Name, "sam@gsi.co.id")
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/zip" || !strings.HasPrefix(body, "PK") {
		t.Fatalf("download %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if res, _ := fetch(http.MethodGet, "/api/chat/backups/..%2F..%2Fetc%2Fpasswd", "sam@gsi.co.id"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("outside file: %d", res.StatusCode)
	}
}
