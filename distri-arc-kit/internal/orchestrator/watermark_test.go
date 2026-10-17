package orchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/orchestrator"
	"distri-arc/internal/store/gen"
)

type sentInput struct {
	Since    string            `json:"since"`
	Dealers  []json.RawMessage `json:"dealers"`
	Messages []struct {
		Dealer uuid.UUID `json:"dealer_uuid"`
	} `json:"messages"`
}

func inputOf(t *testing.T, o *orchestrator.Orchestrator, cid uuid.UUID, agent string) sentInput {
	t.Helper()
	row, err := o.St.Q.GetCycleInput(context.Background(), gen.GetCycleInputParams{CycleID: cid, Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	var x sentInput
	if err := json.Unmarshal(row.Input, &x); err != nil {
		t.Fatal(err)
	}
	return x
}

// Sam 17 Oct: the AI gets only what is new since its last analysis of the agent — the messages after the mark, and the
// dealers with something new or a candidate; the next cycle without new chats sends no message again.
func TestInputOnlySinceLastAnalysis(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `update policies set value = value || '{"mode": "mcp", "require_ai": true}'::jsonb where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	o.MCPWait = time.Minute
	o.OnMCPWait = func(_ context.Context, cid uuid.UUID, _ int64, agents []string, _ time.Duration) time.Duration {
		go func() { // Claude answers every agent
			for _, a := range agents {
				_, _ = o.Submit(context.Background(), cid, a, []byte("[]"), nil)
			}
		}()
		return 0
	}
	first, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	in1 := inputOf(t, o, first.Cycle.ID, "AI Order")
	if len(in1.Messages) == 0 || in1.Since == "" {
		t.Fatalf("first analysis reads the recent chats: %d messages since %q", len(in1.Messages), in1.Since)
	}
	second, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	in2 := inputOf(t, o, second.Cycle.ID, "AI Order")
	if want := first.Cycle.StartedAt.Format(time.RFC3339); in2.Since != want {
		t.Fatalf("since %q, want the first cycle %q", in2.Since, want)
	}
	// (the seed has a few messages later than the test clock: those stay "new")
	if len(in2.Messages) >= len(in1.Messages) || len(in2.Dealers) >= len(in1.Dealers) {
		t.Fatalf("second analysis re-sent %d messages, %d dealers (first %d)", len(in2.Messages), len(in2.Dealers), len(in1.Dealers))
	}
	// a dealer writes after the mark: only that message goes to the next analysis
	var dealer uuid.UUID
	if err := st.Pool.QueryRow(ctx, `insert into signals (kind, dealer_id, contact_id, occurred_at, dedupe_key, summary, payload)
		select 'wa', dealer_id, contact_id, $1, 'wm-test-1', 'Pak, stok DVR 8ch ada?', '{"direction":"in"}' from signals
		where kind = 'wa' and dealer_id is not null limit 1 returning dealer_id`, now.Now().Add(time.Minute)).Scan(&dealer); err != nil {
		t.Fatal(err)
	}
	third, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	in3 := inputOf(t, o, third.Cycle.ID, "AI Order")
	got := false
	for _, m := range in3.Messages {
		got = got || m.Dealer == dealer
	}
	if len(in3.Messages) != len(in2.Messages)+1 || !got {
		t.Fatalf("new message: %+v (before %d)", in3.Messages, len(in2.Messages))
	}
}

// "Batalkan analisis": a cycle waiting for Claude stops within a second and is recorded as cancelled by the person.
func TestCancelWaitingCycle(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `update policies set value = value || '{"mode": "mcp", "require_ai": true}'::jsonb where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	o.MCPWait = time.Minute
	o.OnMCPWait = func(_ context.Context, cid uuid.UUID, _ int64, _ []string, _ time.Duration) time.Duration {
		go func() {
			time.Sleep(200 * time.Millisecond)
			if _, err := st.Q.CancelCycle(context.Background(), gen.CancelCycleParams{ID: cid, By: "Sam"}); err != nil {
				t.Errorf("cancel: %v", err)
			}
		}()
		return 0
	}
	start := time.Now()
	_, err := o.Run(ctx, all, sched)
	var ce orchestrator.CancelledError
	if !errors.As(err, &ce) || ce.By != "Sam" || time.Since(start) > 20*time.Second {
		t.Fatalf("err %v after %s", err, time.Since(start))
	}
	var status, note string
	if err := st.Pool.QueryRow(ctx, `select status, note from cycles order by number desc limit 1`).Scan(&status, &note); err != nil || status != "failed" || note != "Dibatalkan oleh Sam" {
		t.Fatalf("%s %q %v", status, note, err)
	}
}
