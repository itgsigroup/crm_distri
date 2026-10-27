package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeHermes is a Hermes API server: /v1/models and /v1/chat/completions behind the key.
func fakeHermes(t *testing.T, key string, turns *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"hermes-agent"}]}`))
		case "/v1/chat/completions":
			var body struct {
				Messages []struct{ Role, Content string } `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			turns.Store(int32(len(body.Messages)))
			if !strings.HasPrefix(r.Header.Get("X-Hermes-Session-Key"), "orbit-user-") || body.Messages[0].Role != "system" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			last := body.Messages[len(body.Messages)-1].Content
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "Jawaban untuk: " + last}}}})
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

// Without a configured Hermes the chat answers in demo mode; the CEO connects Hermes (key checked), then each user
// gets Hermes' answers with their own earlier turns as context; only the CEO configures; history is per user.
func TestHermesChat(t *testing.T) {
	srv, _ := chatServer(t)
	const ceo, sales, admin = "sam@gsi.co.id", "andi@gsi.co.id", "admin@gsi.co.id"

	if code, _ := post(t, srv.URL+"/api/mcp/hermes/chat", ceo, map[string]any{"text": "Halo"}); code != http.StatusAccepted {
		t.Fatalf("ask: %d", code)
	}
	msgs := waitReply(t, srv, ceo)
	if len(msgs) != 2 || msgs[1].(map[string]any)["demo"] != true || !strings.Contains(msgs[1].(map[string]any)["content"].(string), "Mode demo") {
		t.Fatalf("demo: %v", msgs)
	}

	var turns atomic.Int32
	h := fakeHermes(t, "rahasia-hermes", &turns)
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
	msgs = waitReply(t, srv, ceo)
	last := msgs[len(msgs)-1].(map[string]any)
	if last["status"] != "done" || last["content"] != "Jawaban untuk: Urutkan per cabang" {
		t.Fatalf("reply: %v", last)
	}
	// system + earlier real turn (question + answer; the demo turn is left out) + the new question
	if turns.Load() != 4 {
		t.Fatalf("turns sent: %d", turns.Load())
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
}
