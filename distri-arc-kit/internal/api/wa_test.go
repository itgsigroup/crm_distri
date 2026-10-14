package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"distri-arc/internal/store"
	"distri-arc/internal/wa"
)

// Many numbers, one per user: a new number is linked first (scan in Chat) and reported by WhatsApp, then given to
// a user of the user master; one user holds one number; a sales user reads only their own number's chats.
func TestWANumbersAndChatScope(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	code, cs := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "cs@gsi.co.id", "name": "CS Kantor", "role_key": "admin", "password": "rahasia-panjang-1"})
	if code != http.StatusCreated {
		t.Fatalf("create user: %d %v", code, cs)
	}
	csID := cs["id"].(string)
	// the phone scans: the link is pending, then WhatsApp reports its number
	if _, err := st.Pool.Exec(ctx, "insert into wa_links (session_id, created_by) values ('link-t1', 'sam@gsi.co.id')"); err != nil {
		t.Fatal(err)
	}
	in := wa.NewIngestor(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := in.ProcessStatus(ctx, wa.Status{Session: "link-t1", State: "pairing", QR: "2@qr"}); err != nil {
		t.Fatal(err)
	}
	if code, l := get(t, srv, "/api/wa/links/link-t1", "sam@gsi.co.id"); code != 200 || l["state"] != "pairing" || l["qr_png"] == nil {
		t.Fatalf("pending link %d %v", code, l)
	}
	if err := in.ProcessStatus(ctx, wa.Status{Session: "link-t1", Account: "6281299990000", State: "connected"}); err != nil {
		t.Fatal(err)
	}
	if code, l := get(t, srv, "/api/wa/links/link-t1", "sam@gsi.co.id"); code != 200 || l["state"] != "connected" || l["wa_number"] != "6281299990000" {
		t.Fatalf("linked %d %v", code, l)
	}
	if code, _ := put(t, srv.URL+"/api/wa/numbers/6281299990000/user", "andi@gsi.co.id", map[string]any{"user_id": csID}); code != http.StatusForbidden {
		t.Fatalf("sales gives a number to another user: %d", code)
	}
	if code, out := put(t, srv.URL+"/api/wa/numbers/6281299990000/user", "sam@gsi.co.id", map[string]any{"user_id": csID}); code != 200 || out["label"] != "CS Kantor" {
		t.Fatalf("assign: %d %v", code, out)
	}
	// Andi already holds a number: a second one is refused
	var andiID string
	_ = st.Pool.QueryRow(ctx, "select id from users where email = 'andi@gsi.co.id'").Scan(&andiID)
	if _, err := st.Pool.Exec(ctx, "insert into wa_links (session_id) values ('link-t2')"); err != nil {
		t.Fatal(err)
	}
	if err := in.ProcessStatus(ctx, wa.Status{Session: "link-t2", Account: "6281277770000", State: "connected"}); err != nil {
		t.Fatal(err)
	}
	if code, _ := put(t, srv.URL+"/api/wa/numbers/6281277770000/user", "sam@gsi.co.id", map[string]any{"user_id": andiID}); code != http.StatusConflict {
		t.Fatalf("second number for one user: %d", code)
	}
	// a field sales without a login holds the number straight from the sales master
	var tantri string
	if err := st.Pool.QueryRow(ctx, "insert into sales_users (name, branch, role) values ('Tantri', 'Jakarta', 'sales') returning id").Scan(&tantri); err != nil {
		t.Fatal(err)
	}
	if code, out := put(t, srv.URL+"/api/wa/numbers/6281277770000/user", "sam@gsi.co.id", map[string]any{"sales_id": tantri}); code != 200 || out["label"] != "Tantri" {
		t.Fatalf("assign to sales: %d %v", code, out)
	}
	var main string
	_ = st.Pool.QueryRow(ctx, "select coalesce(wa_number, '') from sales_users where id = $1", tantri).Scan(&main)
	if main != "6281277770000" {
		t.Fatalf("sales main number: %q", main)
	}
	_, users := get(t, srv, "/api/users", "sam@gsi.co.id")
	for _, x := range users["items"].([]any) {
		if m := x.(map[string]any); m["email"] == "cs@gsi.co.id" && (m["wa_number"] != "6281299990000" || m["wa_state"] != "connected") {
			t.Fatalf("user master shows the number: %v", m)
		}
	}
	var session string
	_ = st.Pool.QueryRow(ctx, "select session_id from wa_numbers where wa_number = '6281299990000'").Scan(&session)
	if session != "link-t1" {
		t.Fatalf("number keeps its link session: %q", session)
	}
	_, all := get(t, srv, "/api/wa/status", "sam@gsi.co.id")
	_, own := get(t, srv, "/api/wa/status", "andi@gsi.co.id")
	if n, m := len(all["items"].([]any)), len(own["items"].([]any)); n != 6 || m != 1 {
		t.Fatalf("numbers: ceo %d, andi %d", n, m)
	}
	_, ceo := get(t, srv, "/api/chat/threads", "sam@gsi.co.id")
	_, andi := get(t, srv, "/api/chat/threads", "andi@gsi.co.id")
	ct, at := ceo["items"].([]any), andi["items"].([]any)
	if len(at) == 0 || len(at) >= len(ct) {
		t.Fatalf("threads: ceo %d, andi %d", len(ct), len(at))
	}
	for _, x := range at {
		if x.(map[string]any)["account"] != "6281234504471" {
			t.Fatalf("andi sees another number's chat: %v", x)
		}
	}
	var other string
	for _, x := range ct {
		if m := x.(map[string]any); m["account"] != "6281234504471" {
			other = m["id"].(string)
			break
		}
	}
	if code, _ := get(t, srv, "/api/chat/threads/"+other, "andi@gsi.co.id"); code != http.StatusNotFound {
		t.Fatalf("andi opens another number's chat: %d", code)
	}
	_, f := get(t, srv, "/api/chat/threads?account=6281534509032", "sam@gsi.co.id")
	for _, x := range f["items"].([]any) {
		if x.(map[string]any)["account"] != "6281534509032" {
			t.Fatal("account filter")
		}
	}
}

// ADR 0033: every number belongs to a team (Sales, Admin, Penagihan): a link carries the team chosen when it was
// started, a CEO/admin moves a number to another team, and Chat lists one team's conversations.
func TestWATeams(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	in := wa.NewIngestor(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	link := func(session, number, team string) {
		if _, err := st.Pool.Exec(ctx, "insert into wa_links (session_id, created_by, team) values ($1, 'sam@gsi.co.id', $2)", session, team); err != nil {
			t.Fatal(err)
		}
		if err := in.ProcessStatus(ctx, wa.Status{Session: session, Account: number, State: "connected"}); err != nil {
			t.Fatal(err)
		}
	}
	link("link-admin", "6281211110000", "admin")
	link("link-billing", "6281222220000", "billing")
	team := func(n string) string {
		var v string
		if err := st.Pool.QueryRow(ctx, "select team from wa_numbers where wa_number = $1", n).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if team("6281211110000") != "admin" || team("6281222220000") != "billing" {
		t.Fatal("a linked number did not take its link's team")
	}
	// a re-link of the same number keeps its team
	link("link-admin-2", "6281211110000", "sales")
	if team("6281211110000") != "admin" {
		t.Error("re-link changed the team")
	}
	if code, _ := put(t, srv.URL+"/api/wa/numbers/6281222220000/team", "andi@gsi.co.id", map[string]any{"team": "sales"}); code != http.StatusForbidden {
		t.Fatalf("sales moved a number: %d", code)
	}
	if code, _ := put(t, srv.URL+"/api/wa/numbers/6281222220000/team", "sam@gsi.co.id", map[string]any{"team": "gudang"}); code != http.StatusBadRequest {
		t.Fatalf("unknown team: %d", code)
	}
	if code, _ := put(t, srv.URL+"/api/wa/numbers/6281299999999/team", "sam@gsi.co.id", map[string]any{"team": "admin"}); code != http.StatusNotFound {
		t.Fatalf("unknown number: %d", code)
	}
	// conversations of each team
	for _, x := range []struct{ jid, title, account string }{
		{"6281300000001@s.whatsapp.net", "Tagihan Toko A", "6281222220000"},
		{"6281300000002@s.whatsapp.net", "Admin Toko B", "6281211110000"},
	} {
		if _, err := st.Pool.Exec(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', $1, $2, $3, now())", x.jid, x.title, x.account); err != nil {
			t.Fatal(err)
		}
	}
	titles := func(q string) map[string]string {
		code, out := get(t, srv, "/api/chat/threads?"+q, "sam@gsi.co.id")
		if code != 200 {
			t.Fatalf("threads %s: %d", q, code)
		}
		m := map[string]string{}
		for _, it := range out["items"].([]any) {
			r := it.(map[string]any)
			m[r["title"].(string)] = r["account_team"].(string)
		}
		return m
	}
	if b := titles("team=billing"); len(b) != 1 || b["Tagihan Toko A"] != "billing" {
		t.Errorf("billing team chats %v", b)
	}
	if a := titles("team=admin"); len(a) != 1 || a["Admin Toko B"] != "admin" {
		t.Errorf("admin team chats %v", a)
	}
	if s := titles("team=sales"); s["Tagihan Toko A"] != "" || s["Admin Toko B"] != "" || len(s) == 0 {
		t.Errorf("sales team chats %v", s)
	}
	// moving the number moves its chats to the other team
	if code, out := put(t, srv.URL+"/api/wa/numbers/6281222220000/team", "sam@gsi.co.id", map[string]any{"team": "admin"}); code != 200 || out["team"] != "admin" {
		t.Fatalf("move team: %d %v", code, out)
	}
	if a := titles("team=admin"); len(a) != 2 {
		t.Errorf("admin team after move %v", a)
	}
	code, status := get(t, srv, "/api/wa/status", "sam@gsi.co.id")
	if code != 200 {
		t.Fatal(code)
	}
	for _, it := range status["items"].([]any) {
		if n := it.(map[string]any); n["wa_number"] == "6281211110000" && n["team"] != "admin" {
			t.Errorf("status team %v", n["team"])
		}
	}
}

// ADR 0033: Penagihan (base finance) reads only the WhatsApp number it holds, like a sales; admin reads every number.
func TestPenagihanSeesOnlyOwnNumber(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	code, fin := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "tagih@gsi.co.id", "name": "Rina Penagihan", "role_key": "finance", "password": "rahasia-panjang-1"})
	if code != http.StatusCreated {
		t.Fatalf("create user: %d %v", code, fin)
	}
	in := wa.NewIngestor(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := st.Pool.Exec(ctx, "insert into wa_links (session_id, created_by, team) values ('link-tagih', 'sam@gsi.co.id', 'billing')"); err != nil {
		t.Fatal(err)
	}
	if err := in.ProcessStatus(ctx, wa.Status{Session: "link-tagih", Account: "6281233330000", State: "connected"}); err != nil {
		t.Fatal(err)
	}
	if code, out := put(t, srv.URL+"/api/wa/numbers/6281233330000/user", "sam@gsi.co.id", map[string]any{"user_id": fin["id"]}); code != 200 {
		t.Fatalf("assign: %d %v", code, out)
	}
	var own string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000009@s.whatsapp.net', 'Tagihan Toko C', '6281233330000', now()) returning id").Scan(&own); err != nil {
		t.Fatal(err)
	}
	code, out := get(t, srv, "/api/chat/threads", "tagih@gsi.co.id")
	if code != 200 {
		t.Fatal(code)
	}
	items := out["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != own {
		t.Fatalf("Penagihan sees %d chats, want only its own: %v", len(items), items)
	}
	var other string
	_ = st.Pool.QueryRow(ctx, "select id from chat_threads where account is distinct from '6281233330000' limit 1").Scan(&other)
	if code, _ := get(t, srv, "/api/chat/threads/"+other, "tagih@gsi.co.id"); code != http.StatusNotFound {
		t.Errorf("Penagihan opened another number's chat: %d", code)
	}
	if code, _ := get(t, srv, "/api/chat/threads/"+own, "tagih@gsi.co.id"); code != 200 {
		t.Errorf("Penagihan cannot open its own chat: %d", code)
	}
	_, status := get(t, srv, "/api/wa/status", "tagih@gsi.co.id")
	if nums := status["items"].([]any); len(nums) != 1 || nums[0].(map[string]any)["wa_number"] != "6281233330000" {
		t.Errorf("Penagihan sees numbers %v", nums)
	}
	// admin reads every number
	if _, all := get(t, srv, "/api/chat/threads", "sam@gsi.co.id"); len(all["items"].([]any)) <= 1 {
		t.Error("admin does not see every number's chats")
	}
}

// ADR 0034: in every chat a holding reply ("tunggu sebentar", "saya konfirmasi dulu") is not an answer — the
// conversation stays "Belum dibalas" with its waiting time running until the customer gets one.
func TestHoldingReplyKeepsWaiting(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	for text, want := range map[string]bool{
		"Tunggu sebentar ya pak": true, "Baik bu, saya konfirmasi dulu ya": true, "Saya tanyakan ke gudang dulu nggih": true,
		"Nanti saya kabari ya pak": true, "Sedang kami cek bu": true, "Boleh pak, nanti saya kirim pricelistnya": true,
		"Ready pak 20 pcs": false, "Harga 450rb, saya cek ongkir dulu ya": false, "Bisa dikirim Senin pak": false,
		"Ditunggu orderannya ya bu": false, "Sudah saya cek, stok ada pak": false, "[dokumen Pricelist.pdf] cek dulu ya": false,
	} {
		var got bool
		if err := st.Pool.QueryRow(ctx, "select is_holding_reply($1)", text).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%q holding = %v, want %v", text, got, want)
		}
	}
	var id string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000077@s.whatsapp.net', 'Toko Hold', '6281234504471', now()) returning id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	add := func(wid, dir, body string, ago string) {
		if _, err := st.Pool.Exec(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, $2, $3, $4, now() - $5::interval, 'sent')", id, wid, dir, body, ago); err != nil {
			t.Fatal(err)
		}
	}
	waiting := func() any {
		_, out := get(t, srv, "/api/chat/threads", "sam@gsi.co.id")
		for _, it := range out["items"].([]any) {
			if r := it.(map[string]any); r["id"] == id {
				return r["waiting_since"]
			}
		}
		t.Fatal("thread not listed")
		return nil
	}
	add("H1", "in", "Pak, harga NVR 16ch berapa?", "3 hours")
	add("H2", "out", "Baik pak, saya konfirmasi dulu ya", "2 hours")
	if waiting() == nil {
		t.Fatal("a holding reply cleared the waiting customer")
	}
	add("H3", "out", "NVR 16ch Rp 2.150.000 pak", "1 hour")
	if w := waiting(); w != nil {
		t.Fatalf("answered, still waiting since %v", w)
	}
}

// ADR 0035: the "Belum dibalas" queue holds commerce only — a closing or an auto-reply asks nothing, our own
// auto-reply answers nothing; a supplier is out of the queue and of the analysis; a personal chat leaves the CRM and
// its later messages are not kept; credentials are hidden in Chat.
func TestChatQueueCleanup(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	thread := func(jid, title string) string {
		var id string
		if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', $1, $2, '6281234504471', now()) returning id", jid, title).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	add := func(id, wid, dir, body, ago string) {
		if _, err := st.Pool.Exec(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, $2, $3, $4, now() - $5::interval, 'sent')", id, wid, dir, body, ago); err != nil {
			t.Fatal(err)
		}
	}
	list := func() map[string]map[string]any {
		_, out := get(t, srv, "/api/chat/threads", "sam@gsi.co.id")
		m := map[string]map[string]any{}
		for _, it := range out["items"].([]any) {
			r := it.(map[string]any)
			m[r["id"].(string)] = r
		}
		return m
	}
	closing := thread("6281300000101@s.whatsapp.net", "Toko Tutup")
	add(closing, "Q1", "in", "harga kamera?", "3 hours")
	add(closing, "Q2", "out", "450rb pak", "2 hours")
	add(closing, "Q3", "in", "ok pak siap 🙏", "1 hour")
	auto := thread("6281300000102@s.whatsapp.net", "Toko Auto")
	add(auto, "Q4", "in", "stok NVR ada?", "3 hours")
	add(auto, "Q5", "out", "Terima kasih telah menghubungi CCTVmart, kami akan segera membalas", "3 hours")
	sup := thread("6281300000103@s.whatsapp.net", "Distributor Lain")
	add(sup, "Q6", "in", "barang inden 2 minggu ya", "3 hours")
	priv := thread("6281300000104@s.whatsapp.net", "Mama")
	add(priv, "Q7", "in", "password wifi: Rumah123", "3 hours")

	l := list()
	if l[closing]["waiting_since"] != nil {
		t.Error("a polite closing put the chat back in the queue")
	}
	if l[auto]["waiting_since"] == nil {
		t.Error("our auto-reply counted as an answer")
	}
	if code, out := put(t, srv.URL+"/api/chat/threads/"+sup+"/contact-type", "sam@gsi.co.id", map[string]any{"type": "supplier"}); code != 200 {
		t.Fatalf("supplier: %d %v", code, out)
	}
	if l = list(); l[sup]["contact_type"] != "supplier" {
		t.Errorf("supplier not marked: %v", l[sup])
	}
	_, detail := get(t, srv, "/api/chat/threads/"+priv, "sam@gsi.co.id")
	if b := detail["messages"].([]any)[0].(map[string]any)["body"]; b != "password wifi: ••••••" {
		t.Errorf("credential shown: %v", b)
	}
	if code, out := put(t, srv.URL+"/api/chat/threads/"+priv+"/contact-type", "sam@gsi.co.id", map[string]any{"type": "personal"}); code != 200 || out["removed"] != true {
		t.Fatalf("personal: %d %v", code, out)
	}
	if _, ok := list()[priv]; ok {
		t.Error("personal chat still listed")
	}
	if n := countRows(t, st, "select count(*) from chat_messages where wa_msg_id = 'Q7'"); n != 0 {
		t.Error("personal messages kept")
	}
	in := wa.NewIngestor(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r, err := in.Process(ctx, wa.Message{ID: "Q8", Account: "6281234504471", ChatJID: "6281300000104@s.whatsapp.net", FromNumber: "6281300000104", Text: "sudah makan?", Time: time.Now()})
	if err != nil || r.Stored || r.Reason != wa.DropPersonal {
		t.Errorf("later personal message: %+v %v", r, err)
	}
}

func countRows(t *testing.T, st *store.Store, sql string) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ADR 0037: /wa/analysis carries Claude's Analisa WhatsApp schedule for the label in Pusat kendali; the schedule can
// be paused but not deleted.
func TestWAClaudeLabel(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	_, out := get(t, srv, "/api/wa/analysis", "sam@gsi.co.id")
	c, ok := out["claude"].(map[string]any)
	if !ok || c["enabled"] != true || c["next_run_at"] == nil || c["last"] != nil {
		t.Fatalf("claude block %v", out["claude"])
	}
	if _, err := st.Pool.Exec(ctx, "insert into mcp_schedule_runs (schedule_id, trigger, status, error, finished_at) select id, 'schedule', 'error', 'kunci belum diisi', now() from mcp_schedules where key = 'wa_analysis'"); err != nil {
		t.Fatal(err)
	}
	_, out = get(t, srv, "/api/wa/analysis", "sam@gsi.co.id")
	if last := out["claude"].(map[string]any)["last"].(map[string]any); last["status"] != "error" || last["error"] != "kunci belum diisi" {
		t.Fatalf("last run %v", last)
	}
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/mcp/schedules/"+c["schedule_id"].(string), nil)
	req.Header.Set("X-Dev-User", "sam@gsi.co.id")
	if res, err := http.DefaultClient.Do(req); err == nil && res.StatusCode == http.StatusOK {
		t.Error("the Pusat kendali schedule was deleted")
	}
}
