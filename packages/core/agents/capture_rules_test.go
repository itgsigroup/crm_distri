package agents

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"arc/packages/core/config"
	"arc/packages/core/domain"
	"arc/packages/core/llm"
)

// Stage 03: the capture eval runs without a database on the deterministic provider
// and must reach ≥ 90 % precision and recall.
func TestCaptureEvalFakeProvider(t *testing.T) {
	root := config.RepoRoot()
	r := llm.NewRouter(nil)
	fake := llm.NewFake()
	r.Register(fake)
	r.SetRoute(llm.Light, llm.Route{Provider: "fake"})
	a := &Agents{LLM: r, Fake: fake, Fx: LoadFakeData(filepath.Join(root, "tests", "fixtures"))}
	a.RegisterFakes()
	cases, err := LoadEvalCases(root)
	if err != nil {
		t.Fatal(err)
	}
	rep := a.EvaluateCapture(context.Background(), cases)
	if rep.Precision < 0.9 || rep.Recall < 0.9 {
		t.Fatalf("precision %.1f%% recall %.1f%%: %v", rep.Precision*100, rep.Recall*100, rep.Failures)
	}
}

func extract(dir, thread, text string) CaptureOutput {
	return RuleExtract(CaptureInput{Channel: "wa_message", Direction: dir, ThreadType: thread, Sender: "X", Text: text, OccurredAt: domain.Now()})
}

// Deterministic capture rules: promises vs waiting, and the deliverable named first.
func TestRuleExtractCommitments(t *testing.T) {
	cases := []struct {
		dir, text, who, contains string
	}{
		{"out", "Siap Pak Wijaya, Senin kami kirim revisinya.", "kami", "revisi Senin"},
		{"out", "Baik Bu, proposal revisinya kami kirim besok pagi.", "kami", "proposal"},
		{"out", "Survey selesai, 4 titik tambahan di gudang B. Laporan menyusul 1 Okt.", "kami", "laporan"},
		{"in", "Bu Dewi, kami ambil opsi B. PO menyusul Kamis.", "mereka", "PO Kamis"},
		// Waiting for the customer is not our promise.
		{"out", "Terima kasih Pak Yusuf! Kami tunggu PO-nya. Kick-off Selasa 29 Sep jam 09.00 tetap ya?", "", ""},
		// A customer asking about our deliverable is not their promise.
		{"in", "Mas Andi, revisi penawarannya jadi hari ini ya? Pak Arif mau lihat sebelum rapat Rabu.", "", ""},
	}
	for _, c := range cases {
		out := extract(c.dir, "cust", c.text)
		if c.who == "" {
			if len(out.Commitments) != 0 {
				t.Errorf("%q: unexpected commitment %+v", c.text, out.Commitments)
			}
			continue
		}
		if len(out.Commitments) != 1 || out.Commitments[0].Who != c.who || !strings.Contains(strings.ToLower(out.Commitments[0].Text), strings.ToLower(c.contains)) ||
			out.Commitments[0].Quote == "" || out.Commitments[0].Confidence <= 0 {
			t.Errorf("%q: got %+v, want %s “%s” with quote & confidence", c.text, out.Commitments, c.who, c.contains)
		}
	}
	// Internal groups never yield customer commitments or signals.
	if out := extract("out", "gint", "Senin kami kirim laporan ke Pak Wijaya, ada vendor lain juga."); len(out.Commitments)+len(out.Signals) != 0 {
		t.Errorf("internal group produced %+v", out)
	}
}
