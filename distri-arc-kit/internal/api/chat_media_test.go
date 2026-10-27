package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/outbox"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

// fakeMedia serves REF1 as a JPEG and says REF-GONE expired on WhatsApp.
type fakeMedia struct{ asked []string }

func (f *fakeMedia) Media(_ context.Context, account string, m wa.Media) (io.ReadCloser, error) {
	f.asked = append(f.asked, account+":"+m.Ref)
	if m.Ref == "REF-GONE" {
		return nil, wa.ErrMediaGone
	}
	return io.NopCloser(strings.NewReader("JPEGDATA")), nil
}

var chatNow = time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB)

func mediaServer(t *testing.T, f wa.MediaFetcher) (*httptest.Server, *store.Store) {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	s := api.New(config.Config{Env: "dev"}, st, clock.Fixed(chatNow), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if f != nil {
		s = s.WithMedia(f)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, st
}

func addMessage(t *testing.T, st *store.Store, thread uuid.UUID, waID, dir, status, body string, at time.Time, media *wa.Media) uuid.UUID {
	t.Helper()
	var raw json.RawMessage
	if media != nil {
		raw, _ = json.Marshal(media)
	}
	from := "6281234504471"
	id, err := st.Q.InsertChatMessage(context.Background(), gen.InsertChatMessageParams{ThreadID: &thread, WaMsgID: &waID, Direction: &dir, FromNumber: &from, Body: &body, Media: raw, Status: status, SentAt: at})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func sinarThread(t *testing.T, st *store.Store) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := st.Pool.QueryRow(context.Background(), "select id from chat_threads where seed_key = 'seed:thread:c-sinar'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// ADR 0029: a bubble's media comes from WhatsApp on request through the number that received it, never stored;
// the reference never reaches the browser.
func TestChatMediaStreamsFromWhatsApp(t *testing.T) {
	f := &fakeMedia{}
	srv, st := mediaServer(t, f)
	th := sinarThread(t, st)
	photo := addMessage(t, st, th, "IMG1", "in", "received", "[gambar] kamera rusak", chatNow.Add(-time.Hour), &wa.Media{Kind: "image", Mime: "image/jpeg", Size: 8, Ref: "REF1"})
	old := addMessage(t, st, th, "IMG0", "in", "received", "[gambar]", chatNow.Add(-2*time.Hour), &wa.Media{Kind: "image"})
	doc := addMessage(t, st, th, "DOC1", "in", "received", "[dokumen BoQ.html]", chatNow.Add(-3*time.Hour), &wa.Media{Kind: "document", Mime: "text/html", FileName: "BoQ.html", Ref: "REF2"})
	gone := addMessage(t, st, th, "IMG9", "in", "received", "[gambar]", chatNow.Add(-4*time.Hour), &wa.Media{Kind: "image", Mime: "image/jpeg", Ref: "REF-GONE"})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/chat/threads/"+th.String(), nil)
	req.Header.Set("X-Dev-User", "andi@gsi.co.id")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(raw), "REF1") {
		t.Fatal("the media reference must not reach the browser")
	}
	var detail struct {
		Messages []struct {
			ID    uuid.UUID `json:"id"`
			Media *struct {
				Kind     string `json:"kind"`
				Viewable bool   `json:"viewable"`
			} `json:"media"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &detail)
	views := map[uuid.UUID]bool{}
	for _, m := range detail.Messages {
		if m.Media != nil {
			views[m.ID] = m.Media.Viewable
		}
	}
	if !views[photo] || views[old] {
		t.Fatalf("viewable flags %v", views)
	}

	fetch := func(id uuid.UUID) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/chat/threads/"+th.String()+"/messages/"+id.String()+"/media", nil)
		req.Header.Set("X-Dev-User", "andi@gsi.co.id")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	r := fetch(photo)
	body, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || string(body) != "JPEGDATA" || r.Header.Get("Content-Type") != "image/jpeg" || r.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(r.Header.Get("Cache-Control"), "private") {
		t.Fatalf("photo: %d %q %v", r.StatusCode, body, r.Header)
	}
	if len(f.asked) != 1 || f.asked[0] != "6281234504471:REF1" {
		t.Fatalf("fetched through %v", f.asked)
	}
	// a type the browser could run is only ever a download
	if r := fetch(doc); r.Header.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("document headers %v", r.Header)
	}
	if r := fetch(old); r.StatusCode != http.StatusNotFound {
		t.Fatalf("no reference: %d", r.StatusCode)
	}
	if r := fetch(gone); r.StatusCode != http.StatusGone {
		t.Fatalf("gone: %d", r.StatusCode)
	}
	// another sales user cannot reach this conversation's media
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/chat/threads/"+th.String()+"/messages/"+photo.String()+"/media", nil)
	req.Header.Set("X-Dev-User", "dewi@gsi.co.id")
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusNotFound {
		t.Fatalf("other sales: %v %v", err, res.Status)
	}

	// without the Baileys bridge (fake/Cloud API) the file cannot be fetched
	srv2, st2 := mediaServer(t, nil)
	th2 := sinarThread(t, st2)
	p2 := addMessage(t, st2, th2, "IMG1", "in", "received", "[gambar]", chatNow.Add(-time.Hour), &wa.Media{Kind: "image", Mime: "image/jpeg", Ref: "REF1"})
	if code, _ := sendJSON(t, http.MethodGet, srv2.URL+"/api/chat/threads/"+th2.String()+"/messages/"+p2.String()+"/media", "andi@gsi.co.id", nil); code != http.StatusConflict {
		t.Fatalf("no bridge: %d", code)
	}
}

// Editing like WhatsApp: a queued reply changes in the outbox; a sent text within 15 minutes is a human decision
// that leaves through the outbox as an edit; later, or someone else's message, is refused.
func TestChatEditIsAHumanDecision(t *testing.T) {
	srv, st := mediaServer(t, nil)
	ctx := context.Background()
	th := sinarThread(t, st)
	base := srv.URL + "/api/chat/threads/" + th.String() + "/messages/"
	recent := addMessage(t, st, th, "WSENT1", "out", "sent", "Siap, dikirim Senen", chatNow.Add(-5*time.Minute), nil)
	late := addMessage(t, st, th, "WOLD1", "out", "sent", "Halo", chatNow.Add(-20*time.Minute), nil)
	theirs := addMessage(t, st, th, "WIN1", "in", "received", "Order 10", chatNow.Add(-6*time.Minute), nil)

	if code, _ := sendJSON(t, http.MethodPatch, base+late.String(), "andi@gsi.co.id", map[string]string{"body": "Halo Pak"}); code != http.StatusConflict {
		t.Fatalf("after 15 minutes: %d", code)
	}
	if code, _ := sendJSON(t, http.MethodPatch, base+theirs.String(), "andi@gsi.co.id", map[string]string{"body": "x"}); code != http.StatusConflict {
		t.Fatalf("customer's message: %d", code)
	}
	code, out := sendJSON(t, http.MethodPatch, base+recent.String(), "andi@gsi.co.id", map[string]string{"body": "Siap, dikirim Senin pagi"})
	if code != http.StatusAccepted {
		t.Fatalf("edit: %d %v", code, out)
	}
	if count(t, st, "select count(*) from proposals where id = $1 and kind = 'reply' and status = 'approved' and decided_by is not null", out["proposal_id"]) != 1 {
		t.Fatal("edit not recorded as a human decision")
	}
	if count(t, st, "select count(*) from audit_log where action = 'chat.edit'") != 1 {
		t.Fatal("edit not audited")
	}
	fake := wa.NewFake()
	r := outbox.DefaultRules()
	if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow.Add(time.Minute)), r).Send(ctx, uuid.MustParse(out["outbox_id"].(string))); err != nil {
		t.Fatal(err)
	}
	if s := fake.Sent(); len(s) != 1 || s[0].EditOf != "WSENT1" || s[0].Text != "Siap, dikirim Senin pagi" {
		t.Fatalf("transport got %+v", s)
	}
	if count(t, st, "select count(*) from chat_messages where id = $1 and body = 'Siap, dikirim Senin pagi' and edited_at is not null", recent) != 1 {
		t.Fatal("bubble not updated after the edit went out")
	}

	// an edit whose job runs after the window is refused by the worker, not sent
	code, out = sendJSON(t, http.MethodPatch, base+recent.String(), "andi@gsi.co.id", map[string]string{"body": "Siap Bu"})
	if code != http.StatusAccepted {
		t.Fatalf("second edit: %d", code)
	}
	if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow.Add(20*time.Minute)), r).Send(ctx, uuid.MustParse(out["outbox_id"].(string))); !errors.Is(err, outbox.ErrRefused) || len(fake.Sent()) != 1 {
		t.Fatalf("late edit: %v, sent %d", err, len(fake.Sent()))
	}

	// a reply still queued: the outbox text changes before it goes out
	code, sent := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+th.String()+"/messages", "andi@gsi.co.id", map[string]string{"body": "Besok sore"})
	if code != http.StatusAccepted {
		t.Fatalf("reply: %d", code)
	}
	if code, _ := sendJSON(t, http.MethodPatch, base+sent["message_id"].(string), "andi@gsi.co.id", map[string]string{"body": "Besok pagi"}); code != http.StatusOK {
		t.Fatalf("queued edit: %d", code)
	}
	if count(t, st, "select count(*) from outbox where id = $1 and payload->>'text' = 'Besok pagi'", sent["outbox_id"]) != 1 {
		t.Fatal("queued text not changed")
	}
	if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow), outbox.Rules{}).Send(ctx, uuid.MustParse(sent["outbox_id"].(string))); err != nil {
		t.Fatal(err)
	}
	if s := fake.Sent(); s[len(s)-1].Text != "Besok pagi" {
		t.Fatalf("sent %+v", s[len(s)-1])
	}
}

// Balas (reply) like WhatsApp: a reply quotes a bubble with its WhatsApp id; it leaves through the outbox with the
// quote, and the conversation shows the quoted bubble above it. A bubble not yet sent cannot be quoted.
func TestChatReplyQuotesABubble(t *testing.T) {
	srv, st := mediaServer(t, nil)
	ctx := context.Background()
	th := sinarThread(t, st)
	asked := addMessage(t, st, th, "WQ1", "in", "received", "Ada stok DVR 8 channel?", chatNow.Add(-10*time.Minute), nil)
	url := srv.URL + "/api/chat/threads/" + th.String() + "/messages"
	code, out := sendJSON(t, http.MethodPost, url, "andi@gsi.co.id", map[string]string{"body": "Ada Mbak, 12 unit.", "reply_to": asked.String()})
	if code != http.StatusAccepted {
		t.Fatalf("reply: %d %v", code, out)
	}
	if count(t, st, "select count(*) from outbox where id = $1 and payload->'quote'->>'id' = 'WQ1' and (payload->'quote'->>'from_me')::bool = false", out["outbox_id"]) != 1 {
		t.Fatal("quote not in the outbox payload")
	}
	// the queued reply cannot be quoted yet
	if code, _ := sendJSON(t, http.MethodPost, url, "andi@gsi.co.id", map[string]string{"body": "x", "reply_to": out["message_id"].(string)}); code != http.StatusConflict {
		t.Fatalf("quoting a queued reply: %d", code)
	}
	fake := wa.NewFake()
	if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow), outbox.Rules{}).Send(ctx, uuid.MustParse(out["outbox_id"].(string))); err != nil {
		t.Fatal(err)
	}
	if s := fake.Sent(); len(s) != 1 || s[0].QuotedID != "WQ1" {
		t.Fatalf("transport got %+v", s)
	}
	code, detail := get(t, srv, "/api/chat/threads/"+th.String(), "andi@gsi.co.id")
	if code != http.StatusOK {
		t.Fatal(code)
	}
	var reply map[string]any
	for _, m := range detail["messages"].([]any) {
		if mm := m.(map[string]any); mm["id"] == out["message_id"] {
			reply = mm
		}
	}
	q, _ := reply["quote"].(map[string]any)
	if q == nil || q["id"] != asked.String() || q["body"] != "Ada stok DVR 8 channel?" || q["direction"] != "in" || reply["can_reply"] != true {
		t.Fatalf("reply bubble %v", reply)
	}
}

// Hapus untuk semua orang: a queued reply is cancelled before it goes out; a sent message (≤ 48 hours) is a human
// decision that leaves through the outbox; the bubble shows as deleted without its text; older ones are refused.
// An older account made without a person profile (before Mapping sales linked one) still replies: the profile its
// decision is recorded against is made on the first reply and kept for the next.
func TestChatReplyFromAccountWithoutProfile(t *testing.T) {
	srv, st := mediaServer(t, nil)
	ctx := context.Background()
	th := sinarThread(t, st)
	if _, err := st.Pool.Exec(ctx, "insert into users (email, name, role, role_key, active) values ('granike@gsi.co.id', 'Granike Monika', 'admin', 'admin', true)"); err != nil {
		t.Fatal(err)
	}
	url := srv.URL + "/api/chat/threads/" + th.String() + "/messages"
	for _, body := range []string{"Siap Pak, kami cek dulu.", "Stok ada 12 unit."} {
		if code, out := sendJSON(t, http.MethodPost, url, "granike@gsi.co.id", map[string]string{"body": body}); code != http.StatusAccepted {
			t.Fatalf("reply %q: %d %v", body, code, out)
		}
	}
	if count(t, st, "select count(*) from sales_users where email = 'granike@gsi.co.id'") != 1 ||
		count(t, st, "select count(*) from proposals p join users u on u.sales_user_id = p.decided_by where u.email = 'granike@gsi.co.id' and p.kind = 'reply'") != 2 {
		t.Fatal("reply not recorded against one profile of the account")
	}
}

// A number logged out of its linked device locks its chat history for everyone — the sales holding it and the CEO —
// until it is linked again (Sam 27 Oct).
func TestChatLockedWhileNumberLoggedOut(t *testing.T) {
	srv, st := mediaServer(t, nil)
	ctx := context.Background()
	th := sinarThread(t, st)
	setState := func(state string) {
		t.Helper()
		if tag, err := st.Pool.Exec(ctx, "update wa_numbers set state = $2 where wa_number = (select account from chat_threads where id = $1)", th, state); err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("set %s: %v %v", state, err, tag)
		}
	}
	listed := func(user string) bool {
		t.Helper()
		_, out := get(t, srv, "/api/chat/threads", user)
		for _, x := range out["items"].([]any) {
			if x.(map[string]any)["id"] == th.String() {
				return true
			}
		}
		return false
	}
	path := "/api/chat/threads/" + th.String()
	if code, _ := get(t, srv, path, "andi@gsi.co.id"); code != http.StatusOK || !listed("andi@gsi.co.id") {
		t.Fatalf("linked number: %d", code)
	}
	setState("logged_out")
	if code, _ := get(t, srv, path, "andi@gsi.co.id"); code != http.StatusLocked {
		t.Fatalf("logged out, holder: %d", code)
	}
	if listed("andi@gsi.co.id") {
		t.Fatal("logged out number's chat still listed for its holder")
	}
	if code, _ := get(t, srv, path, "sam@gsi.co.id"); code != http.StatusLocked || listed("sam@gsi.co.id") {
		t.Fatalf("logged out, CEO still reads the chat: %d", code)
	}
	setState("disconnected") // reconnecting by itself: not locked
	if code, _ := get(t, srv, path, "andi@gsi.co.id"); code != http.StatusOK || !listed("sam@gsi.co.id") {
		t.Fatalf("reconnecting number locked: %d", code)
	}
}

func TestChatDeleteForEveryone(t *testing.T) {
	srv, st := mediaServer(t, nil)
	ctx := context.Background()
	th := sinarThread(t, st)
	base := srv.URL + "/api/chat/threads/" + th.String() + "/messages"
	sent := addMessage(t, st, th, "WDEL1", "out", "sent", "Harga salah, abaikan", chatNow.Add(-2*time.Hour), nil)
	old := addMessage(t, st, th, "WDEL0", "out", "sent", "Lama", chatNow.Add(-72*time.Hour), nil)
	theirs := addMessage(t, st, th, "WDELIN", "in", "received", "Order 3", chatNow.Add(-time.Hour), nil)

	if code, _ := sendJSON(t, http.MethodDelete, base+"/"+old.String(), "andi@gsi.co.id", nil); code != http.StatusConflict {
		t.Fatalf("after 48 hours: %d", code)
	}
	if code, _ := sendJSON(t, http.MethodDelete, base+"/"+theirs.String(), "andi@gsi.co.id", nil); code != http.StatusConflict {
		t.Fatalf("customer's message: %d", code)
	}
	code, out := sendJSON(t, http.MethodDelete, base+"/"+sent.String(), "andi@gsi.co.id", nil)
	if code != http.StatusAccepted || count(t, st, "select count(*) from audit_log where action = 'chat.delete'") != 1 {
		t.Fatalf("delete: %d %v", code, out)
	}
	fake := wa.NewFake()
	if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow), outbox.Rules{}).Send(ctx, uuid.MustParse(out["outbox_id"].(string))); err != nil {
		t.Fatal(err)
	}
	if s := fake.Sent(); len(s) != 1 || s[0].RevokeOf != "WDEL1" {
		t.Fatalf("transport got %+v", s)
	}

	// a reply still queued: cancelled, never sent
	_, q := sendJSON(t, http.MethodPost, base, "andi@gsi.co.id", map[string]string{"body": "Salah kirim"})
	if code, _ := sendJSON(t, http.MethodDelete, base+"/"+q["message_id"].(string), "andi@gsi.co.id", nil); code != http.StatusOK {
		t.Fatalf("queued delete: %d", code)
	}
	if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow), outbox.Rules{}).Send(ctx, uuid.MustParse(q["outbox_id"].(string))); err != nil || len(fake.Sent()) != 1 {
		t.Fatalf("cancelled reply went out: %v %d", err, len(fake.Sent()))
	}

	_, detail := get(t, srv, "/api/chat/threads/"+th.String(), "andi@gsi.co.id")
	deleted := 0
	for _, m := range detail["messages"].([]any) {
		mm := m.(map[string]any)
		if mm["deleted"] == true {
			deleted++
			if mm["body"] != "" || mm["can_reply"] == true {
				t.Errorf("deleted bubble shows %v", mm)
			}
		}
	}
	if deleted != 2 {
		t.Fatalf("%d deleted bubbles", deleted)
	}
}

func sendFile(t *testing.T, url, user, name, ctype string, data []byte, fields map[string]string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	h.Set("Content-Type", ctype)
	part, _ := mw.CreatePart(h)
	_, _ = part.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Dev-User", user)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// Kirim file, pesan suara, polling (ADR 0031): the file waits in chat_outgoing_files until the outbox sends it, then
// is deleted and the bubble keeps WhatsApp's download reference; a poll goes out as a poll.
func TestChatSendsFilesVoiceNotesAndPolls(t *testing.T) {
	srv, st := mediaServer(t, nil)
	ctx := context.Background()
	th := sinarThread(t, st)
	url := srv.URL + "/api/chat/threads/" + th.String() + "/messages"
	fake := wa.NewFake()
	send := func(ob string, at time.Duration) {
		t.Helper()
		if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow.Add(at)), outbox.Rules{}).Send(ctx, uuid.MustParse(ob)); err != nil {
			t.Fatal(err)
		}
	}

	code, out := sendFile(t, url, "andi@gsi.co.id", "stok.jpg", "image/jpeg", []byte("\xff\xd8\xff\xe0JPEGDATA"), map[string]string{"body": "Stok kamera di gudang"})
	if code != http.StatusAccepted {
		t.Fatalf("photo: %d %v", code, out)
	}
	if count(t, st, "select count(*) from chat_outgoing_files") != 1 ||
		count(t, st, "select count(*) from chat_messages where id = $1 and body = '[gambar] Stok kamera di gudang' and media->>'kind' = 'image'", out["message_id"]) != 1 {
		t.Fatal("photo not queued")
	}
	// the photo shows as it was sent while it waits in the queue (no WhatsApp fetch: this server has none)
	mid := out["message_id"].(string)
	photo := func(when string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, url+"/"+mid+"/media", nil)
		req.Header.Set("X-Dev-User", "andi@gsi.co.id")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" || string(b) != "\xff\xd8\xff\xe0JPEGDATA" {
			t.Fatalf("photo %s: %d %q %q", when, res.StatusCode, res.Header.Get("Content-Type"), b)
		}
		_, page := get(t, srv, "/api/chat/threads/"+th.String(), "andi@gsi.co.id")
		for _, m := range page["messages"].([]any) {
			if mm := m.(map[string]any); mm["id"] == mid && mm["media"].(map[string]any)["viewable"] != true {
				t.Fatalf("photo %s: bubble not viewable", when)
			}
		}
	}
	photo("queued")
	send(out["outbox_id"].(string), 0)
	if s := fake.Sent(); len(s) != 1 || s[0].Media == nil || s[0].Media.Kind != "image" || s[0].Text != "Stok kamera di gudang" {
		t.Fatalf("transport got %+v", s)
	}
	photo("sent")
	if count(t, st, "select count(*) from chat_outgoing_files") != 1 ||
		count(t, st, "select count(*) from chat_messages where id = $1 and status = 'sent' and media->>'ref' like 'FAKEREF-%' and body = '[gambar] Stok kamera di gudang'", out["message_id"]) != 1 {
		t.Fatal("sent photo: file not kept or reference missing")
	}

	code, out = sendFile(t, url, "andi@gsi.co.id", "vn.webm", "audio/webm;codecs=opus", []byte("WEBMOPUS"), map[string]string{"voice": "1", "seconds": "7"})
	if code != http.StatusAccepted {
		t.Fatalf("voice note: %d %v", code, out)
	}
	send(out["outbox_id"].(string), time.Minute)
	if s := fake.Sent(); s[len(s)-1].Media == nil || !s[len(s)-1].Media.PTT || s[len(s)-1].Media.Seconds != 7 {
		t.Fatalf("voice note sent as %+v", s[len(s)-1])
	}

	code, out = sendJSON(t, http.MethodPost, url, "andi@gsi.co.id", map[string]any{"poll": map[string]any{"name": "Kirim kapan?", "options": []string{"Senin", "Selasa", "Senin"}}})
	if code != http.StatusAccepted {
		t.Fatalf("poll: %d %v", code, out)
	}
	send(out["outbox_id"].(string), 2*time.Minute)
	if s := fake.Sent(); s[len(s)-1].Media == nil || s[len(s)-1].Media.Kind != "poll" {
		t.Fatalf("poll sent as %+v", s[len(s)-1])
	}
	if code, _ := sendJSON(t, http.MethodPost, url, "andi@gsi.co.id", map[string]any{"poll": map[string]any{"name": "x", "options": []string{"satu"}}}); code != http.StatusBadRequest {
		t.Fatalf("poll with one option: %d", code)
	}

	// a WhatsApp contact card (Sam 17 Oct): name + number, a local 08… number becomes 62…
	code, out = sendJSON(t, http.MethodPost, url, "andi@gsi.co.id", map[string]any{"contact": map[string]any{"name": " Pak  Budi ", "phone": "0812-3456-7890"}})
	if code != http.StatusAccepted || count(t, st, "select count(*) from chat_messages where id = $1 and body = '[kontak] Pak Budi · +6281234567890' and media->>'kind' = 'contact'", out["message_id"]) != 1 {
		t.Fatalf("contact: %d %v", code, out)
	}
	send(out["outbox_id"].(string), 3*time.Minute)
	if s := fake.Sent(); s[len(s)-1].Media == nil || s[len(s)-1].Media.Kind != "contact" {
		t.Fatalf("contact sent as %+v", s[len(s)-1])
	}
	if code, _ := sendJSON(t, http.MethodPost, url, "andi@gsi.co.id", map[string]any{"contact": map[string]any{"name": "Budi", "phone": "123"}}); code != http.StatusBadRequest {
		t.Fatalf("contact without a valid number: %d", code)
	}
	big := make([]byte, 25<<20+10)
	if code, _ := sendFile(t, url, "andi@gsi.co.id", "besar.pdf", "application/pdf", big, nil); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("file over 25 MB: %d", code)
	}
}

// Forwarding a bubble from one conversation to another is a reply in the target: a human decision through the
// outbox (audited as chat.forward). A photo is fetched from WhatsApp and sent on as a file with its caption; a
// conversation the person cannot see cannot be forwarded from.
func TestChatForward(t *testing.T) {
	srv, st := mediaServer(t, &fakeMedia{})
	ctx := context.Background()
	src := sinarThread(t, st)
	var dst uuid.UUID
	if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, sales_id, account, last_message_at)
		select 'new', '6281300000301@s.whatsapp.net', 'Toko Tujuan', sales_id, account, now() from chat_threads where id = $1 returning id`, src).Scan(&dst); err != nil {
		t.Fatal(err)
	}
	text := addMessage(t, st, src, "FW1", "in", "received", "Mohon kirim 10 unit IPC ke gudang Bekasi", chatNow.Add(-time.Hour), nil)
	photo := addMessage(t, st, src, "FW2", "in", "received", "[gambar] Lokasi pasang", chatNow.Add(-2*time.Hour), &wa.Media{Kind: "image", Mime: "image/jpeg", Size: 8, Ref: "REF1"})
	url := srv.URL + "/api/chat/threads/" + dst.String() + "/messages"
	fake := wa.NewFake()
	send := func(ob any, at time.Duration) wa.Message {
		t.Helper()
		if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow.Add(at)), outbox.Rules{}).Send(ctx, uuid.MustParse(ob.(string))); err != nil {
			t.Fatal(err)
		}
		s := fake.Sent()
		return s[len(s)-1]
	}
	fw := func(id uuid.UUID, user string) (int, map[string]any) {
		return sendJSON(t, http.MethodPost, url, user, map[string]any{"forward": map[string]string{"thread_id": src.String(), "message_id": id.String()}})
	}

	code, out := fw(text, "andi@gsi.co.id")
	if code != http.StatusAccepted {
		t.Fatalf("forward text: %d %v", code, out)
	}
	if m := send(out["outbox_id"], 0); m.Text != "Mohon kirim 10 unit IPC ke gudang Bekasi" || m.Media != nil {
		t.Fatalf("forwarded text sent as %+v", m)
	}
	if count(t, st, "select count(*) from audit_log where action = 'chat.forward' and after->'forwarded_from'->>'message_id' = $1", text.String()) != 1 ||
		count(t, st, "select count(*) from proposals where id = $1 and decided_by is not null and title like 'Teruskan ke %'", out["proposal_id"]) != 1 {
		t.Fatal("forward not recorded as a human decision")
	}

	code, out = fw(photo, "andi@gsi.co.id")
	if code != http.StatusAccepted {
		t.Fatalf("forward photo: %d %v", code, out)
	}
	if m := send(out["outbox_id"], time.Minute); m.Media == nil || m.Media.Kind != "image" || m.Text != "Lokasi pasang" {
		t.Fatalf("forwarded photo sent as %+v", m)
	}
	if count(t, st, "select count(*) from chat_messages where id = $1 and body = '[gambar] Lokasi pasang'", out["message_id"]) != 1 {
		t.Fatal("forwarded photo bubble")
	}

	// another sales user sees neither conversation
	if code, _ := fw(text, "dewi@gsi.co.id"); code != http.StatusNotFound {
		t.Fatalf("other sales forwarded: %d", code)
	}
}

// A nickname given in GSI Orbit is also saved in the WhatsApp contacts of the number holding the chat (Sam 17 Oct).
func TestAliasSavedToPhoneContacts(t *testing.T) {
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	fake := wa.NewFake()
	s := api.New(config.Config{Env: "dev"}, st, clock.Fixed(chatNow), slog.New(slog.NewTextHandler(io.Discard, nil))).WithContacts(fake)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	th := sinarThread(t, st)
	var account, jid string
	if err := st.Pool.QueryRow(context.Background(), "select coalesce(account, ''), coalesce(wa_jid, '') from chat_threads where id = $1", th).Scan(&account, &jid); err != nil {
		t.Fatal(err)
	}
	code, out := put(t, srv.URL+"/api/chat/threads/"+th.String()+"/alias", "sam@gsi.co.id", map[string]any{"name": "Mbak Rina Sinar"})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if account == "" {
		if out["phone_synced"] != false {
			t.Fatalf("no account, still synced: %v", out)
		}
		return
	}
	if out["phone_synced"] != true || len(fake.Saved()) != 1 || fake.Saved()[0] != account+":"+wa.NumberOfJID(jid)+"=Mbak Rina Sinar" {
		t.Fatalf("synced %v saved %v", out, fake.Saved())
	}
}

// 19 Oct bug: a chat of Dewi's dealer that sits on Andi's number goes out from Andi's number — and must be shown as
// sent by Andi (the number's holder), not as Dewi (the chat's sales), both on the bubble and as "Dikirim dari".
func TestReplyNamedAfterTheNumbersHolder(t *testing.T) {
	srv, st := mediaServer(t, &fakeMedia{})
	ctx := context.Background()
	src := sinarThread(t, st)
	var account string
	if err := st.Pool.QueryRow(ctx, "select account from chat_threads where id = $1", src).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into wa_numbers (wa_number, session_id, label, transport, user_id)
		values ($1, $1, '', 'fake', (select id from users where email = 'andi@gsi.co.id'))
		on conflict (wa_number) do update set label = '', user_id = excluded.user_id`, account); err != nil {
		t.Fatal(err)
	}
	var dst uuid.UUID
	if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, sales_id, account, last_message_at)
		values ('dealer', '6281300000302@s.whatsapp.net', 'Toko Dewi', (select id from sales_users where name = 'Dewi'), $1, now()) returning id`, account).Scan(&dst); err != nil {
		t.Fatal(err)
	}
	text := addMessage(t, st, src, "FW9", "in", "received", "Nomor mba Elisa: 0812 0000 9999", chatNow.Add(-time.Hour), nil)
	code, out := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+dst.String()+"/messages", "sam@gsi.co.id",
		map[string]any{"forward": map[string]string{"thread_id": src.String(), "message_id": text.String()}})
	if code != http.StatusAccepted {
		t.Fatalf("forward: %d %v", code, out)
	}
	var name, holder string
	if err := st.Pool.QueryRow(ctx, "select from_name from chat_messages where id = $1", out["message_id"]).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "select name from users where email = 'andi@gsi.co.id'").Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if name != holder || name == "Dewi" {
		t.Fatalf("sent as %q, want the number's holder %q", name, holder)
	}
	code, th := get(t, srv, "/api/chat/threads/"+dst.String(), "sam@gsi.co.id")
	if tv, _ := th["thread"].(map[string]any); code != http.StatusOK || tv["sender_name"] != holder {
		t.Fatalf("thread sender %d %v", code, th["thread"])
	}
	// the sender label: who pressed Teruskan, for everyone who reads the chat (Sam 22 Oct: one sales number, several
	// admins — each bubble names the user logged in when it was sent)
	var sam string
	_ = st.Pool.QueryRow(ctx, "select name from users where email = 'sam@gsi.co.id'").Scan(&sam)
	sentBy := func(body map[string]any) any {
		for _, x := range body["messages"].([]any) {
			if m := x.(map[string]any); m["id"] == out["message_id"] {
				return m["sent_by"]
			}
		}
		return "missing"
	}
	if got := sentBy(th); got != sam {
		t.Fatalf("CEO sees sent_by %v, want %q", got, sam)
	}
	// the sales holding the number sees the chat even though its sales profile is Dewi's — with the sender label too
	code, own := get(t, srv, "/api/chat/threads/"+dst.String(), "andi@gsi.co.id")
	if code != http.StatusOK {
		t.Fatalf("holder of the number cannot open the chat on it: %d", code)
	}
	if got := sentBy(own); got != sam {
		t.Fatalf("sales sees sent_by %v, want %q", got, sam)
	}
}
