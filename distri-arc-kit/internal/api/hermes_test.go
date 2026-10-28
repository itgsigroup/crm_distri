package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHermes is a Hermes API server: /v1/models and /v1/responses behind the key. Like Hermes it keeps each named
// conversation itself; seen records what Orbit sent.
type hermesTurn struct{ Conversation, SessionKey, Input string }

func fakeHermes(t *testing.T, key string, seen *[]hermesTurn) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"hermes-agent"}]}`))
		case "/v1/responses":
			var body struct {
				Input        any    `json:"input"`
				Conversation string `json:"conversation"`
				Store        bool   `json:"store"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			in, ok := body.Input.(string)
			if !ok || body.Conversation == "" || !body.Store {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			*seen = append(*seen, hermesTurn{body.Conversation, r.Header.Get("X-Hermes-Session-Key"), in})
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "response", "status": "completed", "output": []any{
				map[string]any{"type": "function_call", "name": "dealer_list"},
				map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Jawaban untuk: " + in}}},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// waitReply polls the chat until no reply is pending and returns the messages.
func waitReply(t *testing.T, srv *httptest.Server, user string) []any {
	t.Helper()
	for range 100 {
		_, out := get(t, srv, "/api/mcp/hermes/chat", user)
		msgs, _ := out["messages"].([]any)
		pending := false
		for _, m := range msgs {
			if m.(map[string]any)["status"] == "pending" {
				pending = true
			}
		}
		if !pending {
			return msgs
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("reply still pending")
	return nil
}

// The Hermes page is a window to Lyra: without a saved Hermes nothing is asked (no demo); the CEO connects Hermes
// (key checked); each question goes to Lyra alone (no Orbit prompt or history) in a conversation Lyra keeps, one per
// user, and Hapus riwayat starts a new one; only the CEO configures; history is per user.
func TestHermesChat(t *testing.T) {
	srv, _ := chatServer(t)
	const ceo, sales, admin = "sam@gsi.co.id", "andi@gsi.co.id", "admin@gsi.co.id"

	if code, out := post(t, srv.URL+"/api/mcp/hermes/chat", ceo, map[string]any{"text": "Halo"}); code != http.StatusConflict || !strings.Contains(fmt.Sprint(out), "not_connected") {
		t.Fatalf("ask without Lyra: %d %v", code, out)
	}
	if _, out := get(t, srv, "/api/mcp/hermes/chat", ceo); len(out["messages"].([]any)) != 0 {
		t.Fatalf("stored a question nobody answers: %v", out["messages"])
	}

	var seen []hermesTurn
	h := fakeHermes(t, "rahasia-hermes", &seen)
	if code, _ := put(t, srv.URL+"/api/mcp/hermes/config", ceo, map[string]any{"url": h.URL + "/v1/", "api_key": "salah"}); code != http.StatusBadRequest {
		t.Fatalf("wrong key accepted: %d", code)
	}
	if code, _ := put(t, srv.URL+"/api/mcp/hermes/config", sales, map[string]any{"url": h.URL, "api_key": "rahasia-hermes"}); code != http.StatusForbidden {
		t.Fatalf("sales configured Hermes: %d", code)
	}
	code, cfg := put(t, srv.URL+"/api/mcp/hermes/config", ceo, map[string]any{"url": h.URL + "/v1/", "api_key": "rahasia-hermes"})
	if code != http.StatusOK || cfg["configured"] != true || cfg["url"] != h.URL || cfg["key_hint"] != "••••rmes" {
		t.Fatalf("config: %d %v", code, cfg)
	}

	post(t, srv.URL+"/api/mcp/hermes/chat", ceo, map[string]any{"text": "Dealer mana yang lewat jadwal?"})
	waitReply(t, srv, ceo)
	post(t, srv.URL+"/api/mcp/hermes/chat", ceo, map[string]any{"text": "Urutkan per cabang"})
	msgs := waitReply(t, srv, ceo)
	last := msgs[len(msgs)-1].(map[string]any)
	if len(msgs) != 4 || last["status"] != "done" || last["content"] != "Jawaban untuk: Urutkan per cabang" {
		t.Fatalf("reply: %v", msgs)
	}
	if len(seen) != 2 || seen[1].Input != "Urutkan per cabang" || seen[0].Conversation != seen[1].Conversation ||
		!strings.HasPrefix(seen[0].Conversation, "orbit-") || !strings.HasPrefix(seen[0].SessionKey, "orbit-user-") {
		t.Fatalf("sent to Lyra: %+v", seen)
	}

	// the chat follows the MCP screen; a role without it gets 403, another user with it sees only their own chat
	if code, _ := get(t, srv, "/api/mcp/hermes/chat", sales); code != http.StatusForbidden {
		t.Fatalf("sales without the MCP screen: %d", code)
	}
	if code, other := get(t, srv, "/api/mcp/hermes/chat", admin); code == http.StatusOK && len(other["messages"].([]any)) != 0 {
		t.Fatalf("admin sees the CEO's chat: %v", other["messages"])
	}
	if code, _ := del(t, srv.URL+"/api/mcp/hermes/chat", ceo); code != http.StatusNoContent {
		t.Fatalf("clear: %d", code)
	}
	if _, out := get(t, srv, "/api/mcp/hermes/chat", ceo); len(out["messages"].([]any)) != 0 {
		t.Fatal("history not cleared")
	}
	post(t, srv.URL+"/api/mcp/hermes/chat", ceo, map[string]any{"text": "Halo lagi"})
	waitReply(t, srv, ceo)
	if len(seen) != 3 || seen[2].Conversation == seen[0].Conversation || seen[2].SessionKey != seen[0].SessionKey {
		t.Fatalf("after clearing, a new conversation: %+v", seen)
	}
}
