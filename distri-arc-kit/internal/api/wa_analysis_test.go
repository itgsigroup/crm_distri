package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/store/gen"
)

func waDealers(out map[string]any) map[string]map[string]any {
	m := map[string]map[string]any{}
	for _, x := range out["items"].([]any) {
		it := x.(map[string]any)
		if d, ok := it["dealer_id"].(string); ok {
			m[d] = it
		}
	}
	return m
}

// GET /wa/analysis (ADR 0026): the last cycle's reading of WhatsApp, scoped like Pusat kendali (a sales sees their
// conversations, ?sales= one sales' page), guarded by the chat page, and the waiting state read now.
func TestWAAnalysis(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()

	_, empty := get(t, srv, "/api/wa/analysis", "sam@gsi.co.id")
	if empty["source"] != "none" || len(empty["items"].([]any)) != 0 {
		t.Fatalf("before any cycle: %v", empty)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	o := &orchestrator.Orchestrator{St: st, Clock: clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)), Router: &llm.Router{Primary: llm.NewFake(nil), St: st, Log: log}, Log: log}
	if _, err := o.Run(ctx, domain.Scope{Kind: "all"}, domain.Trigger{Source: "schedule"}); err != nil {
		t.Fatal(err)
	}

	code, all := get(t, srv, "/api/wa/analysis", "sam@gsi.co.id")
	if code != http.StatusOK || all["cycle_number"] == nil || all["source"] != "rules" {
		t.Fatalf("CEO: %d %v", code, all)
	}
	graha := waDealers(all)["graha"]
	if graha == nil || graha["needs_reply"] != true || graha["sales"] == "" {
		t.Fatalf("graha: %v", graha)
	}
	if n := all["totals"].(map[string]any)["threads"].(float64); int(n) != len(all["items"].([]any)) || len(all["history"].([]any)) != 1 {
		t.Errorf("totals %v history %v", all["totals"], all["history"])
	}

	// a sales user sees only their own conversations; another sales' page keeps theirs
	_, andi := get(t, srv, "/api/wa/analysis", "andi@gsi.co.id")
	if waDealers(andi)["graha"] != nil {
		t.Error("Andi sees Fajar's Graha conversation")
	}
	_, fajarPage := get(t, srv, "/api/wa/analysis?sales="+graha["sales"].(string), "sam@gsi.co.id")
	if waDealers(fajarPage)["graha"] == nil || fajarPage["overview"] != nil {
		t.Errorf("Fajar's page: %v", fajarPage["items"])
	}
	// Penagihan (finance) reads only the conversations of the number it holds: none here, and no overview of everyone
	if code, fin := get(t, srv, "/api/wa/analysis", "finance@gsi.co.id"); code != 200 || len(fin["items"].([]any)) != 0 || fin["overview"] != nil {
		t.Errorf("finance sees other numbers' conversations: %d %v", code, fin["items"])
	}

	// the plan step of the conversation (the seed has Pak Hasan's next message at 10.12: the reply comes after it) is on Fajar's page, not Andi's, and done once the reply went out
	step := func(user, q string) map[string]any {
		_, plan := get(t, srv, "/api/plan/today"+q, user)
		for _, x := range plan["items"].([]any) {
			if it := x.(map[string]any); it["agent"] == orchestrator.AgentWA {
				return it
			}
		}
		return nil
	}
	if step("sam@gsi.co.id", "?sales="+graha["sales"].(string)) == nil || step("andi@gsi.co.id", "") != nil {
		t.Error("Balas chat step scoped wrong")
	}
	if s := step("sam@gsi.co.id", ""); s == nil || s["status"] != "scheduled" {
		t.Fatalf("step %v", s)
	}
	tid := uuid.MustParse(graha["thread_id"].(string))
	dir, body, id := "out", "Siap Pak.", "test-reply-graha"
	if _, err := st.Q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &tid, WaMsgID: &id, Direction: &dir, Body: &body, SentAt: time.Date(2026, 10, 5, 10, 30, 0, 0, clock.WIB), Status: "sent"}); err != nil {
		t.Fatal(err)
	}
	if s := step("sam@gsi.co.id", ""); s == nil || s["status"] != "done" {
		t.Errorf("answered step %v", s)
	}
	_, after := get(t, srv, "/api/wa/analysis", "sam@gsi.co.id")
	if g := waDealers(after)["graha"]; g["answered"] != true || g["needs_reply"] != false {
		t.Errorf("graha after the reply: %v", g)
	}
}
