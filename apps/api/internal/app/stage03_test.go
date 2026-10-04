package app

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"

	"arc/packages/connectors/whatsapp"
	"arc/packages/core/agents"
	"arc/packages/core/config"
	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/llm"
)

func interactionID(t *testing.T, a *App, thread, text string) int64 {
	t.Helper()
	var id int64
	if err := a.DB.Pool.QueryRow(context.Background(), `SELECT id FROM interactions WHERE thread_id=$1 AND body_text=$2`, thread, text).Scan(&id); err != nil {
		t.Fatalf("fixture message %q in %s: %v", text, thread, err)
	}
	return id
}

func evidenceHas(t *testing.T, raw []byte, interaction int64) bool {
	t.Helper()
	var ev []domain.Evidence
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("evidence: %v", err)
	}
	for _, e := range ev {
		if e.InteractionID == interaction && e.Quote != "" {
			return true
		}
	}
	return false
}

// Stage 03: Capture on the fixture chats finds the commitments, the competitor
// signal and the crane task, each with evidence + confidence; re-running adds nothing.
func TestStage03CaptureFixtureChats(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	threads := []string{"c-wijaya", "c-yusuf", "c-hendra", "g-simpanglima", "c-kartika", "g-pelabuhan", "g-teknisi"}
	reset := func() {
		exec(t, a, `UPDATE interactions SET extracted=false, extraction_version=0 WHERE thread_id = ANY($1)`, threads)
	}
	reset()
	for _, th := range threads {
		if _, err := a.Agents.ExtractThread(ctx, th); err != nil {
			t.Fatalf("extract %s: %v", th, err)
		}
	}
	revisi := interactionID(t, a, "c-wijaya", "Siap Pak Wijaya, Senin kami kirim revisinya.")
	po := interactionID(t, a, "c-yusuf", "Bu Dewi, kami ambil opsi B. PO menyusul Kamis.")
	comp := interactionID(t, a, "c-hendra", "Bu Dewi, ada vendor lain menawarkan harga lebih rendah untuk spek setara. Apakah masih ada ruang penyesuaian?")
	crane := interactionID(t, a, "g-simpanglima", "Bisa Pak, tim 4 orang. Butuh crane kecil, saya koordinasi vendor.")

	checkCommitment := func(who, acc, pattern string, origin int64) {
		var text string
		var ev []byte
		var conf float64
		err := a.DB.Pool.QueryRow(ctx, `SELECT text, evidence, confidence::float8 FROM commitments WHERE who=$1 AND account_id=$2 AND text ILIKE $3 AND origin_interaction_id=$4`,
			who, acc, pattern, origin).Scan(&text, &ev, &conf)
		if err != nil {
			t.Fatalf("commitment %s %q on %s from interaction %d: %v", who, pattern, acc, origin, err)
		}
		if conf <= 0 || !evidenceHas(t, ev, origin) {
			t.Fatalf("commitment %q lacks evidence/confidence: conf=%v ev=%s", text, conf, ev)
		}
	}
	checkCommitment("kami", "semarang", "revisi%senin%", revisi)
	checkCommitment("mereka", "bsd", "PO%kamis%", po)

	var sev []byte
	var sconf float64
	if err := a.DB.Pool.QueryRow(ctx, `SELECT evidence, confidence::float8 FROM signals WHERE type='competitor_mentioned' AND account_id='rsud' AND resolved_at IS NULL`).Scan(&sev, &sconf); err != nil {
		t.Fatalf("competitor signal on rsud: %v", err)
	}
	if sconf <= 0 || !evidenceHas(t, sev, comp) {
		t.Fatalf("competitor signal without evidence from the Hendra message: conf=%v ev=%s", sconf, sev)
	}

	var title, assignee string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT title, assignee FROM tasks WHERE source_interaction_id=$1 AND title ILIKE '%crane%'`, crane).Scan(&title, &assignee); err != nil {
		t.Fatalf("crane task: %v", err)
	}
	if !strings.HasPrefix(strings.ToLower(title), "sewa crane") || !strings.Contains(assignee, "Bayu") {
		t.Fatalf("crane task = %q → %q, want “sewa crane → Bayu”", title, assignee)
	}
	var aev []byte
	var aconf float64
	if err := a.DB.Pool.QueryRow(ctx, `SELECT evidence, confidence::float8 FROM extractions WHERE interaction_id=$1 AND kind='task'`, crane).Scan(&aev, &aconf); err != nil || aconf <= 0 || !evidenceHas(t, aev, crane) {
		t.Fatalf("crane annotation evidence/confidence missing: %v %s", err, aev)
	}
	// Internal group: tasks/notes only, never customer commitments or signals.
	if n := count(t, a, `SELECT count(*) FROM commitments c JOIN interactions i ON i.id=c.origin_interaction_id WHERE i.thread_id='g-teknisi'`); n != 0 {
		t.Fatalf("internal group produced %d commitments", n)
	}

	snapshot := func() [4]int {
		return [4]int{count(t, a, `SELECT count(*) FROM commitments`), count(t, a, `SELECT count(*) FROM signals`),
			count(t, a, `SELECT count(*) FROM tasks`), count(t, a, `SELECT count(*) FROM extractions`)}
	}
	// Drain every other pending interaction first so only re-extraction is measured.
	if _, err := a.Agents.ExtractPending(ctx, 1000); err != nil {
		t.Fatal(err)
	}
	before := snapshot()
	reset()
	if _, err := a.Agents.ExtractPending(ctx, 1000); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("re-extraction duplicated rows: before %v after %v (commitments, signals, tasks, extractions)", before, after)
	}
}

// Stage 03: inbound identification — Rudi identified with research, the agency is
// not a prospect, the number without any source stays unknown.
func TestStage03InboundIdentification(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	// Forget the seeded results so the agent recomputes them from the source chain.
	exec(t, a, `UPDATE inbound_contacts SET status='unknown', fit_score=NULL, overview='', solutions='[]', pain_questions='[]',
		identification = identification - 'name' - 'company' - 'role' - 'model' WHERE id IN ('in1','in3','in4')`)

	out, err := a.Agents.IdentifyInbound(ctx, "in1")
	if err != nil {
		t.Fatal(err)
	}
	var status, company string
	var sols, qs []map[string]any
	var solRaw, qRaw []byte
	if err := a.DB.Pool.QueryRow(ctx, `SELECT status, identification->>'company', solutions, pain_questions FROM inbound_contacts WHERE id='in1'`).Scan(&status, &company, &solRaw, &qRaw); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(solRaw, &sols)
	_ = json.Unmarshal(qRaw, &qs)
	if status != "identified" || company != "PT Prima Karya Sejahtera" || out.Company != company {
		t.Fatalf("Rudi: status=%s company=%q", status, company)
	}
	if len(sols) < 4 || len(qs) != 5 {
		t.Fatalf("Rudi research: %d solutions (want ≥ 4), %d questions (want 5)", len(sols), len(qs))
	}

	if _, err := a.Agents.IdentifyInbound(ctx, "in4"); err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, a, "in4"); s != "not_prospect" {
		t.Fatalf("agency: %s, want not_prospect", s)
	}
	if _, err := a.Agents.IdentifyInbound(ctx, "in3"); err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, a, "in3"); s != "unknown" {
		t.Fatalf("empty number: %s, want unknown", s)
	}

	// A brand-new inbound number through the WhatsApp path: contact recorded, nothing stored
	// (odoo_contacts_only), identification without sources → unknown.
	ev := whatsapp.WaEvent{Wamid: "3EB0NEWNUM01", Session: "s-andi", ChatID: "6289900112233@s.whatsapp.net", From: "6289900112233", Text: "Halo",
		Timestamp: domain.Now(), Transport: "fake"}
	r, err := a.IngestWaEvent(ctx, ev)
	if err != nil || r.InboundID == "" || r.Stored {
		t.Fatalf("new inbound number: %+v %v", r, err)
	}
	if _, err := a.Agents.IdentifyInbound(ctx, r.InboundID); err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, a, r.InboundID); s != "unknown" {
		t.Fatalf("unknown new number: %s", s)
	}
}

func statusOf(t *testing.T, a *App, id string) string {
	var s string
	if err := a.DB.Pool.QueryRow(context.Background(), `SELECT status FROM inbound_contacts WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Stage 03: the capture eval set reaches ≥ 90 % precision and recall with the FakeProvider.
func TestStage03CaptureEval(t *testing.T) {
	a, _ := fresh(t)
	cases, err := agents.LoadEvalCases(config.RepoRoot())
	if err != nil || len(cases) < 15 {
		t.Fatalf("eval cases: %d %v", len(cases), err)
	}
	r := a.Agents.EvaluateCapture(context.Background(), cases)
	if r.Provider != "fake" {
		t.Fatalf("eval must run on the fake provider, got %s", r.Provider)
	}
	if r.Precision < 0.9 || r.Recall < 0.9 {
		t.Fatalf("precision %.1f%% recall %.1f%% (TP %d FP %d FN %d): %v", r.Precision*100, r.Recall*100, r.TP, r.FP, r.FN, r.Failures)
	}
}

// externalSpy stands in for an external provider (data leaves the server, so the
// router must mask PII) while answering with the deterministic fake.
type externalSpy struct{ inner llm.Provider }

func (s externalSpy) Name() string   { return "anthropic" }
func (s externalSpy) External() bool { return true }
func (s externalSpy) Complete(ctx context.Context, model string, req llm.Request) (llm.Response, error) {
	resp, err := s.inner.Complete(ctx, model, req)
	resp.Provider, resp.Model = "anthropic", model
	return resp, err
}

// Independent detector (not the masker's own regex): any Indonesian mobile number,
// with or without separators after the country code.
var rawPhone = regexp.MustCompile(`(?:\+?62|\b0)[\s.\-]?8\d{1,3}[\s.\-]?\d{3,4}[\s.\-]?\d{3,5}`)

// Stage 03: no raw phone number reaches an external provider; usage is logged and
// GET /api/llm/usage reports it.
func TestStage03PIIInterceptAndUsage(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	a.LLM.Register(externalSpy{inner: a.Fake})
	for _, tier := range []llm.Tier{llm.Light, llm.Heavy, llm.Interactive} {
		a.LLM.SetRoute(tier, llm.Route{Provider: "anthropic", Model: "claude-haiku-4-5"})
	}
	var mu sync.Mutex
	var payloads []string
	a.LLM.Payloads = func(provider string, req llm.Request) {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, req.System)
		for _, m := range req.Messages {
			payloads = append(payloads, m.Content)
		}
	}
	// Capture of a message carrying phone numbers in several formats.
	ev := whatsapp.WaEvent{Wamid: "3EB0PII00001", Session: "s-andi", ChatID: phWijaya + "@s.whatsapp.net", From: phWijaya, SenderName: "Pak Wijaya",
		Text: "Mas, hubungi Pak Arif di +62 813-2900-7310 atau 0812 2955 1120, revisi penawaran Senin ya.", Timestamp: domain.Now(), Transport: "fake"}
	if r, err := a.IngestWaEvent(ctx, ev); err != nil || !r.Stored {
		t.Fatalf("ingest: %+v %v", r, err)
	}
	if _, err := a.Agents.ExtractThread(ctx, "c-wijaya"); err != nil {
		t.Fatal(err)
	}
	// Identification (the payload includes the inbound phone) and Ask.
	if _, err := a.Agents.IdentifyInbound(ctx, "in1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Agents.Ask(ctx, "sam", "Deal mana yang berisiko?", "ask", insights.Scope{All: true}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(payloads) < 3 {
		t.Fatalf("expected provider payloads, got %d", len(payloads))
	}
	sawToken := false
	for _, p := range payloads {
		if m := rawPhone.FindString(p); m != "" {
			t.Fatalf("raw phone %q reached the external provider in: %.300s", m, p)
		}
		if strings.Contains(p, "[PHONE_") {
			sawToken = true
		}
	}
	if !sawToken {
		t.Fatal("no masked phone token seen — the intercept did not see the PII-bearing payloads")
	}
	if count(t, a, `SELECT count(*) FROM llm_calls WHERE provider='anthropic' AND input_hash <> '' AND purpose IN ('capture','identity_overview','ask')`) < 3 {
		t.Fatal("LLM calls not logged")
	}
	var usage struct {
		ByModel []struct {
			Provider string `json:"provider"`
			Calls    int    `json:"calls"`
		} `json:"by_model"`
		Routes map[string]map[string]string `json:"routes"`
	}
	login(t, srv, "sam@gsi.co.id").json("GET", "/api/llm/usage", nil, 200, &usage)
	found := false
	for _, m := range usage.ByModel {
		if m.Provider == "anthropic" && m.Calls >= 3 {
			found = true
		}
	}
	if !found || usage.Routes["light"]["provider"] != "anthropic" {
		t.Fatalf("usage report: %+v", usage)
	}
}
