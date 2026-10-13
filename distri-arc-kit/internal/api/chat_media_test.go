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
	send(out["outbox_id"].(string), 0)
	if s := fake.Sent(); len(s) != 1 || s[0].Media == nil || s[0].Media.Kind != "image" || s[0].Text != "Stok kamera di gudang" {
		t.Fatalf("transport got %+v", s)
	}
	if count(t, st, "select count(*) from chat_outgoing_files") != 0 ||
		count(t, st, "select count(*) from chat_messages where id = $1 and status = 'sent' and media->>'ref' like 'FAKEREF-%' and body = '[gambar] Stok kamera di gudang'", out["message_id"]) != 1 {
		t.Fatal("sent photo: file kept or reference missing")
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
	big := make([]byte, 25<<20+10)
	if code, _ := sendFile(t, url, "andi@gsi.co.id", "besar.pdf", "application/pdf", big, nil); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("file over 25 MB: %d", code)
	}
}
