package api_test

import (
	"context"
	"net/http"
	"testing"
)

// Pesan berbintang: each person's own — Sam's star is in Sam's "Berbintang" list and on the bubble for Sam, not for
// Andi; unstarring removes it; a deleted message cannot be starred.
func TestStarredMessages(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var thread string
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where seed_key = 'seed:thread:c-sinar'").Scan(&thread); err != nil {
		t.Fatal(err)
	}
	var older, newer string
	if err := st.Pool.QueryRow(ctx, "select id from chat_messages where thread_id = $1 order by sent_at limit 1", thread).Scan(&older); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status)
		values ($1, 'STAR-NEW', 'in', 'kirim besok ya', now(), 'received') returning id`, thread).Scan(&newer); err != nil {
		t.Fatal(err)
	}
	star := func(mid, user string, on bool) int {
		code, _ := sendJSON(t, http.MethodPut, srv.URL+"/api/chat/threads/"+thread+"/messages/"+mid+"/star", user, map[string]any{"starred": on})
		return code
	}
	list := func(user string) []string {
		_, out := get(t, srv, "/api/chat/starred", user)
		var ids []string
		for _, x := range out["items"].([]any) {
			ids = append(ids, x.(map[string]any)["id"].(string))
		}
		return ids
	}
	if star(older, "sam@gsi.co.id", true) != 200 || star(newer, "sam@gsi.co.id", true) != 200 || star(newer, "sam@gsi.co.id", true) != 200 {
		t.Fatal("star")
	}
	if ids := list("sam@gsi.co.id"); len(ids) != 2 || ids[0] != newer || ids[1] != older {
		t.Fatalf("Sam's starred, newest first: %v", ids)
	}
	if ids := list("andi@gsi.co.id"); len(ids) != 0 {
		t.Fatalf("Andi sees Sam's stars: %v", ids)
	}
	starredOn := func(user string) map[string]bool {
		_, out := get(t, srv, "/api/chat/threads/"+thread, user)
		m := map[string]bool{}
		for _, x := range out["messages"].([]any) {
			b := x.(map[string]any)
			m[b["id"].(string)] = b["starred"] == true
		}
		return m
	}
	if s := starredOn("sam@gsi.co.id"); !s[newer] {
		t.Fatalf("bubble not starred for Sam: %v", s)
	}
	if s := starredOn("andi@gsi.co.id"); s[newer] {
		t.Fatal("bubble starred for Andi")
	}
	if star(newer, "sam@gsi.co.id", false) != 200 {
		t.Fatal("unstar")
	}
	if ids := list("sam@gsi.co.id"); len(ids) != 1 || ids[0] != older {
		t.Fatalf("after unstar: %v", ids)
	}
	if _, err := st.Pool.Exec(ctx, "update chat_messages set deleted_at = now() where id = $1", newer); err != nil {
		t.Fatal(err)
	}
	if code := star(newer, "sam@gsi.co.id", true); code != http.StatusBadRequest {
		t.Fatalf("deleted message starred: %d", code)
	}
}
