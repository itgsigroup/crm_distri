package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

type fakeLIDs struct{}

func (fakeLIDs) Media(context.Context, string, wa.Media) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// The device does not know 215030000000001@lid itself: WhatsApp gives it as the LID of 6282335180530 when that number
// is asked (a candidate: a chat with the same name).
func (fakeLIDs) PhonesForLIDs(_ context.Context, _ string, lids, pns []string) (map[string]string, error) {
	out := map[string]string{}
	for _, l := range lids {
		if l == "215030000000001@lid" && slices.Contains(pns, "6282335180530@s.whatsapp.net") {
			out[l] = "6282335180530@s.whatsapp.net"
		}
	}
	return out, nil
}

// ADR 0072: loading the chat list merges a LID chat into the number's chat in the background (the list shows one
// "Mas Rizal IT - GSI", not two); a LID the device does not know stays.
func TestChatListMergesLIDChats(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.New(config.Config{Env: "dev"}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).WithMedia(fakeLIDs{}).Handler())
	t.Cleanup(srv.Close)
	const andi = "6281234504471"
	for _, jid := range []string{"6282335180530@s.whatsapp.net", "215030000000001@lid", "215030000000009@lid"} {
		if _, err := st.Pool.Exec(ctx, `with t as (insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', $1, 'rizal fachrezi', $2, now()) returning id)
			insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) select id, $1, 'in', 'test', now(), 'received' from t`, jid, andi); err != nil {
			t.Fatal(err)
		}
	}
	count := func(jid string) int {
		var n int
		_ = st.Pool.QueryRow(ctx, "select count(*) from chat_threads where account = $1 and wa_jid = $2", andi, jid).Scan(&n)
		return n
	}
	get(t, srv, "/api/chat/threads?account="+andi, "sam@gsi.co.id")
	for i := 0; count("215030000000001@lid") != 0; i++ {
		if i > 200 {
			t.Fatal("the LID chat was not merged")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var msgs int
	_ = st.Pool.QueryRow(ctx, "select count(*) from chat_messages m join chat_threads t on t.id = m.thread_id where t.wa_jid = '6282335180530@s.whatsapp.net'").Scan(&msgs)
	if msgs != 2 || count("215030000000009@lid") != 1 {
		t.Fatalf("after merge: %d messages on the number's chat, unknown LID chat %d", msgs, count("215030000000009@lid"))
	}
}
