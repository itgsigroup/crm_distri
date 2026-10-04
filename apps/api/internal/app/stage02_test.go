package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"arc/packages/connectors/whatsapp"
	"arc/packages/core/domain"
)

const (
	phWijaya       = "6281229015521"
	phKartika      = "6281228113311"
	phDimas        = "6281390112233"
	phYusuf        = "6281227776611"
	phHendra       = "6281126501442"
	phBayu         = "6285722111188" // internal (Teknisi Semarang)
	phGudang       = "6281133000045" // internal (Gudang Surabaya)
	phAndi         = "6281231044471"
	jidSimpangLima = "120363041001@g.us"
	jidTeknisi     = "120363041003@g.us"
	jidGudang      = "120363041004@g.us"
	jidNewExtGroup = "120363099999@g.us"
	internalSecret = "INTERNAL-1on1 rahasia: jadwal cuti & gaji"
)

func waEvent(i int, session, chat, from, sender, text string, group bool) whatsapp.WaEvent {
	return whatsapp.WaEvent{Wamid: fmt.Sprintf("3EB0LIVE%04d", i), Session: session, ChatID: chat, From: from, SenderName: sender, Text: text,
		IsGroup: group, Timestamp: demoAnchor.Add(-time.Duration(60-i) * time.Minute), Transport: "fake"}
}

// replayEvents is the Stage 02 fixture replay: 10 customer 1:1, 8 external group,
// 4 internal group and 2 internal 1:1 events.
func replayEvents() (cust, ext, extNoOpt, gint, gintNoOpt, int1 []whatsapp.WaEvent) {
	i := 0
	next := func() int { i++; return i }
	c := func(s, ph, name, text string) whatsapp.WaEvent {
		return waEvent(next(), s, ph+"@s.whatsapp.net", ph, name, text, false)
	}
	cust = []whatsapp.WaEvent{
		c("s-andi", phWijaya, "Pak Wijaya", "Mas Andi, struktur sudah bisa dikirim minggu ini?"),
		c("s-andi", phWijaya, "Pak Wijaya", "Pak Arif minta jadwal pemasangan."),
		c("s-andi", phWijaya, "Pak Wijaya", "Terima kasih, ditunggu kabarnya."),
		c("s-andi", phKartika, "Bu Kartika", "Mas, invoice signage bisa dikirim ke email keuangan?"),
		c("s-andi", phKartika, "Bu Kartika", "Sekalian jadwal training operator ya."),
		c("s-andi", phDimas, "Pak Dimas", "Mas Andi, auditorium jadi direnovasi tahun depan."),
		c("s-dewi", phYusuf, "Pak Yusuf", "Bu Dewi, PO sudah ditandatangani Bu Dian."),
		c("s-dewi", phYusuf, "Pak Yusuf", "Kick-off besok jam 9 tetap."),
		c("s-dewi", phHendra, "Pak Hendra", "Bu Dewi, rapat internal kami geser ke Kamis."),
		c("s-dewi", phHendra, "Pak Hendra", "Mohon kirim ulang lampiran rev.2."),
	}
	meta := &whatsapp.GroupMeta{Name: "Proyek Videotron Simpang Lima", Members: []whatsapp.GroupMember{
		{Phone: phAndi, Name: "Andi"}, {Phone: phWijaya, Name: "Pak Wijaya"}, {Phone: phBayu, Name: "Bayu Pratama"}}}
	g := func(s, chat, ph, name, text string, m *whatsapp.GroupMeta) whatsapp.WaEvent {
		ev := waEvent(next(), s, chat, ph, name, text, true)
		ev.GroupMeta = m
		return ev
	}
	for _, x := range [][2]string{{phWijaya, "Struktur tiang tiba Kamis pagi."}, {phBayu, "Siap, tim 4 orang standby."}, {phWijaya, "Izin galian aman."},
		{phBayu, "Butuh scaffold tambahan, saya koordinasi."}, {phWijaya, "Foto progres tolong dikirim sore."}, {phBayu, "Oke Pak, nanti sore."}} {
		ext = append(ext, g("s-andi", jidSimpangLima, x[0], "", x[1], meta))
	}
	newMeta := &whatsapp.GroupMeta{Name: "Grup baru pelanggan", Members: []whatsapp.GroupMember{{Phone: phAndi, Name: "Andi"}, {Phone: "6281299990000", Name: "Pak Baru"}}}
	extNoOpt = []whatsapp.WaEvent{
		g("s-andi", jidNewExtGroup, "6281299990000", "Pak Baru", "EXT-NOOPT pesan pertama", newMeta),
		g("s-andi", jidNewExtGroup, "6281299990000", "Pak Baru", "EXT-NOOPT pesan kedua", newMeta),
	}
	gint = []whatsapp.WaEvent{
		g("s-andi", jidTeknisi, phBayu, "", "Besok survey Kendal jam 9.", nil),
		g("s-andi", jidTeknisi, phBayu, "", "Yang ikut: Dedi dan saya.", nil),
	}
	gintNoOpt = []whatsapp.WaEvent{
		g("s-rizky", jidGudang, phGudang, "", "GINT-NOOPT stok kabel habis", nil),
		g("s-rizky", jidGudang, phGudang, "", "GINT-NOOPT order minggu depan", nil),
	}
	int1 = []whatsapp.WaEvent{
		c("s-andi", phBayu, "Bayu Pratama", internalSecret),
		c("s-andi", phBayu, "Bayu Pratama", internalSecret+" (2)"),
	}
	return
}

// Stage 02: replay through IngestWaEvent applies the privacy rules and is idempotent.
func TestStage02ReplayPrivacyAndIdempotency(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	cust, ext, extNoOpt, gint, gintNoOpt, int1 := replayEvents()
	all := append(append(append(append(append(append([]whatsapp.WaEvent{}, cust...), ext...), extNoOpt...), gint...), gintNoOpt...), int1...)
	if len(all) != 24 || len(cust) != 10 || len(ext)+len(extNoOpt) != 8 || len(gint)+len(gintNoOpt) != 4 || len(int1) != 2 {
		t.Fatalf("replay set must be 10/8/4/2 = 24 events, got %d", len(all))
	}
	gudangSkipped := count(t, a, `SELECT skipped_count FROM chat_groups WHERE jid=$1`, jidGudang)
	interactionsBefore := count(t, a, `SELECT count(*) FROM interactions`)

	ingest := func(evs []whatsapp.WaEvent) []IngestResult {
		var out []IngestResult
		for _, ev := range evs {
			r, err := a.IngestWaEvent(ctx, ev)
			if err != nil {
				t.Fatalf("ingest %s: %v", ev.Wamid, err)
			}
			out = append(out, r)
		}
		return out
	}
	for _, r := range ingest(cust) {
		if !r.Stored {
			t.Fatalf("customer message not stored: %+v", r)
		}
	}
	for _, r := range ingest(ext) {
		if !r.Stored || r.ThreadID != "g-simpanglima" {
			t.Fatalf("external opted-in group message not stored in g-simpanglima: %+v", r)
		}
	}
	for _, r := range ingest(extNoOpt) {
		if r.Stored || r.Skipped != "group_not_opted_in" {
			t.Fatalf("non-opt-in external group stored: %+v", r)
		}
	}
	for _, r := range ingest(gint) {
		if !r.Stored || r.ThreadID != "g-teknisi" {
			t.Fatalf("internal opted-in group message not stored: %+v", r)
		}
	}
	for _, r := range ingest(gintNoOpt) {
		if r.Stored || r.Skipped != "group_not_opted_in" {
			t.Fatalf("non-opt-in internal group stored: %+v", r)
		}
	}
	for _, r := range ingest(int1) {
		if r.Stored || r.Skipped != "internal_private" {
			t.Fatalf("internal 1:1 stored: %+v", r)
		}
	}
	stored := count(t, a, `SELECT count(*) FROM interactions`) - interactionsBefore
	if stored != 18 {
		t.Fatalf("stored %d interactions, want 18 (10 cust + 6 ext + 2 gint)", stored)
	}
	// Threads & classification.
	if n := count(t, a, `SELECT count(*) FROM interactions i JOIN chat_threads t ON t.id=i.thread_id WHERE i.wamid LIKE '3EB0LIVE%' AND t.type='cust' AND i.person_ids <> '{}' AND i.account_id IS NOT NULL`); n != 10 {
		t.Fatalf("customer messages linked to person+account in cust threads: %d, want 10", n)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE wamid LIKE '3EB0LIVE%' AND thread_id='c-wijaya' AND account_id='semarang'`); n != 3 {
		t.Fatalf("Pak Wijaya messages in c-wijaya: %d", n)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE wamid LIKE '3EB0LIVE%' AND channel='wa_group_message' AND thread_id='g-simpanglima'`); n != 6 {
		t.Fatalf("external group messages: %d", n)
	}
	if n := count(t, a, `SELECT count(*) FROM chat_groups WHERE jid=$1 AND type='external' AND NOT read_policy AND skipped_count=2`, jidNewExtGroup); n != 1 {
		t.Fatal("new external group must be classified external, not opted in, with 2 skipped messages counted")
	}
	if got := count(t, a, `SELECT skipped_count FROM chat_groups WHERE jid=$1`, jidGudang); got != gudangSkipped+2 {
		t.Fatalf("internal non-opt-in group counter %d, want %d", got, gudangSkipped+2)
	}
	if n := count(t, a, `SELECT count(*) FROM chat_groups WHERE jid=$1 AND type='internal'`, jidTeknisi); n != 1 {
		t.Fatal("Teknisi Semarang must stay internal")
	}
	// Privacy: no content of internal 1:1 chats or non-opt-in groups anywhere.
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE body_text LIKE 'INTERNAL-1on1%' OR body_text LIKE '%NOOPT%'`); n != 0 {
		t.Fatalf("%d private/non-opt-in message bodies stored", n)
	}
	if n := count(t, a, `SELECT count(*) FROM chat_threads WHERE chat_jid=$1 AND is_private AND type='internal' AND skipped_count >= 2`, phBayu+"@s.whatsapp.net"); n != 1 {
		t.Fatal("internal 1:1 must leave only a private marker thread")
	}
	// Replay again → zero new interactions.
	for _, ev := range all {
		r, err := a.IngestWaEvent(ctx, ev)
		if err != nil {
			t.Fatal(err)
		}
		if r.Stored {
			t.Fatalf("replay stored a duplicate: %s", ev.Wamid)
		}
	}
	if n := count(t, a, `SELECT count(*) FROM interactions`) - interactionsBefore; n != 18 {
		t.Fatalf("replay duplicated interactions: %d", n)
	}
	// Replay must not inflate the "not read" counters either.
	if got := count(t, a, `SELECT skipped_count FROM chat_groups WHERE jid=$1`, jidGudang); got != gudangSkipped+2 {
		t.Fatalf("replay changed internal group skip counter: %d, want %d", got, gudangSkipped+2)
	}
	if n := count(t, a, `SELECT count(*) FROM chat_groups WHERE jid=$1 AND skipped_count=2`, jidNewExtGroup); n != 1 {
		t.Fatal("replay changed the external group skip counter")
	}
	if n := count(t, a, `SELECT count(*) FROM chat_threads WHERE chat_jid=$1 AND skipped_count=2`, phBayu+"@s.whatsapp.net"); n != 1 {
		t.Fatal("replay changed the internal 1:1 skip counter")
	}
}

// Stage 02: the bridge webhook requires a valid HMAC; the bridge's approval
// check refuses actions without a human decision; transports refuse sends without an action id.
func TestStage02BridgeWebhookSignature(t *testing.T) {
	a, srv := fresh(t)
	ev := waEvent(1, "s-andi", phWijaya+"@s.whatsapp.net", phWijaya, "Pak Wijaya", "Halo dari webhook", false)
	body, _ := json.Marshal(map[string]any{"type": "message", "event": ev})
	post := func(sig string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/webhooks/wa", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if sig != "" {
			req.Header.Set("X-ARC-Signature", sig)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("sha256=" + whatsapp.Sign("wrong-secret", body)); code != 401 {
		t.Fatalf("bad signature → %d, want 401", code)
	}
	if code := post(""); code != 401 {
		t.Fatalf("missing signature → %d, want 401", code)
	}
	if count(t, a, `SELECT count(*) FROM interactions WHERE body_text='Halo dari webhook'`) != 0 {
		t.Fatal("unsigned event was stored")
	}
	if code := post("sha256=" + whatsapp.Sign(a.Cfg.BridgeSecret, body)); code != 200 {
		t.Fatalf("valid signature → %d", code)
	}
	if count(t, a, `SELECT count(*) FROM interactions WHERE body_text='Halo dari webhook'`) != 1 {
		t.Fatal("signed event not stored")
	}

	check := func(id, sig string) (int, map[string]any) {
		req, _ := http.NewRequest("GET", srv.URL+"/bridge/actions/"+id, nil)
		req.Header.Set("X-ARC-Signature", sig)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if code, _ := check("unmer", whatsapp.Sign("wrong", []byte("unmer"))); code != 401 {
		t.Fatalf("bridge action check with bad HMAC → %d", code)
	}
	code, out := check("unmer", whatsapp.Sign(a.Cfg.BridgeSecret, []byte("unmer")))
	if code != 200 || out["approved"] != false {
		t.Fatalf("proposed action must not be approved for the bridge: %d %v", code, out)
	}
	if _, err := a.FakeWA.Send(context.Background(), "s-andi", phDimas+"@s.whatsapp.net", "x", ""); err == nil {
		t.Fatal("transport sent without an approved action id")
	}
}

// Stage 02: a WhatsApp chat export imports as history without duplicating live events
// (live → export and export → live), and re-importing is a no-op.
func TestStage02ExportDedupesAgainstLive(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	andi := login(t, srv, "andi@gsi.co.id")
	chat := phWijaya + "@s.whatsapp.net"
	// A live event received from the bridge (real WhatsApp message id, seconds precision).
	liveAt := time.Date(2026, 9, 27, 10, 15, 42, 0, domain.Jakarta)
	live := whatsapp.WaEvent{Wamid: "3EB0C0FFEE0001", Session: "s-andi", ChatID: chat, From: phWijaya, SenderName: "Pak Wijaya",
		Text: "Mas Andi, gambar kerja struktur sudah saya terima.", Timestamp: liveAt, Transport: "bridge"}
	if r, err := a.IngestWaEvent(ctx, live); err != nil || !r.Stored {
		t.Fatalf("live event: %+v %v", r, err)
	}
	export := strings.Join([]string{
		"20/07/26 09.01 - Pak Wijaya: Selamat pagi Mas Andi, kami sedang menyusun kebutuhan videotron.",
		"20/07/26 09.05 - Andi: Pagi Pak, siap kami bantu. Ukuran yang dibutuhkan berapa?",
		"20/07/26 09.07 - Pak Wijaya: Kira-kira 6×4 m, lokasi Simpang Lima.",
		"27/09/26 10.15 - Pak Wijaya: Mas Andi, gambar kerja struktur sudah saya terima.",
		"28/09/26 08.30 - Pak Wijaya: Mas, nanti siang saya telepon soal jadwal.",
	}, "\n")
	path := "/api/wa/import?" + url.Values{"session": {"s-andi"}, "chat_jid": {chat}}.Encode()
	var res struct {
		Parsed, Stored, Duplicates int
	}
	andi.json("POST", path, []byte(export), 200, &res)
	if res.Parsed != 5 || res.Stored != 4 || res.Duplicates != 1 {
		t.Fatalf("first import: %+v (want parsed 5, stored 4, duplicates 1 — the live message)", res)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE thread_id='c-wijaya' AND body_text=$1`, live.Text); n != 1 {
		t.Fatalf("live message present %d times after export import", n)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE thread_id='c-wijaya' AND is_history AND transport='export'`); n != 4 {
		t.Fatalf("history rows: %d", n)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE thread_id='c-wijaya' AND is_history AND direction='out' AND body_text LIKE 'Pagi Pak, siap%'`); n != 1 {
		t.Fatal("own messages in the export must be outbound")
	}
	// Re-import → nothing new.
	andi.json("POST", path, []byte(export), 200, &res)
	if res.Stored != 0 || res.Duplicates != 5 {
		t.Fatalf("re-import: %+v", res)
	}
	// Export → live: the bridge later delivers a message already imported.
	// Its push name differs from the contact name saved on the sales phone.
	late := whatsapp.WaEvent{Wamid: "3EB0C0FFEE0002", Session: "s-andi", ChatID: chat, From: phWijaya, SenderName: "Wijaya S.",
		Text: "Mas, nanti siang saya telepon soal jadwal.", Timestamp: time.Date(2026, 9, 28, 8, 30, 12, 0, domain.Jakarta), Transport: "bridge"}
	if r, err := a.IngestWaEvent(ctx, late); err != nil || r.Stored || !r.Duplicate {
		t.Fatalf("live copy of an exported message: %+v %v", r, err)
	}
}
