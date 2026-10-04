package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"arc/packages/core/domain"
	"arc/packages/core/llm"
	"arc/packages/core/prompts"
)

// EvalCase is one capture evaluation case.
type EvalCase struct {
	ID             string `json:"id"`
	ThreadType     string `json:"thread_type"`
	Direction      string `json:"direction"`
	Sender         string `json:"sender"`
	SenderInternal bool   `json:"sender_internal"`
	Text           string `json:"text"`
	Expect         struct {
		Commitments []struct {
			Who      string `json:"who"`
			Contains string `json:"contains"`
		} `json:"commitments"`
		Signals []string `json:"signals"`
		Tasks   []struct {
			Contains string `json:"contains"`
			Assignee string `json:"assignee"`
		} `json:"tasks"`
	} `json:"expect"`
}

// EvalReport is the precision/recall summary.
type EvalReport struct {
	Provider   string
	Cases      int
	TP, FP, FN int
	Precision  float64
	Recall     float64
	Failures   []string
}

// Score returns the F-style accuracy used by the acceptance gate (min of precision and recall).
func (r EvalReport) Score() float64 {
	if r.Precision < r.Recall {
		return r.Precision
	}
	return r.Recall
}

// LoadEvalCases reads tests/eval/capture_cases.json.
func LoadEvalCases(root string) ([]EvalCase, error) {
	raw, err := os.ReadFile(filepath.Join(root, "tests", "eval", "capture_cases.json"))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Cases []EvalCase `json:"cases"`
	}
	return doc.Cases, json.Unmarshal(raw, &doc)
}

// EvaluateCapture runs the cases through the router tier used by Capture.
func (a *Agents) EvaluateCapture(ctx context.Context, cases []EvalCase) EvalReport {
	r := EvalReport{Provider: a.LLM.RouteFor(llm.Light).Provider, Cases: len(cases)}
	for _, c := range cases {
		in := CaptureInput{Channel: "wa_message", Direction: c.Direction, ThreadType: c.ThreadType, Sender: c.Sender, SenderInternal: c.SenderInternal, Text: c.Text, OccurredAt: domain.Now()}
		raw, _ := json.Marshal(in)
		var out CaptureOutput
		if _, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Light, Purpose: "capture", System: prompts.Get("capture/v1"),
			Messages: []llm.Message{{Role: "user", Content: string(raw)}}, Schema: captureSchema, MaxTokens: 4000, FakeInput: in}, &out); err != nil {
			r.FN += len(c.Expect.Commitments) + len(c.Expect.Signals) + len(c.Expect.Tasks)
			r.Failures = append(r.Failures, c.ID+": "+err.Error())
			continue
		}
		matched := map[int]bool{}
		for _, e := range c.Expect.Commitments {
			ok := false
			for i, got := range out.Commitments {
				if !matched[i] && got.Who == e.Who && strings.Contains(strings.ToLower(got.Text), strings.ToLower(e.Contains)) {
					matched[i], ok = true, true
					break
				}
			}
			if ok {
				r.TP++
			} else {
				r.FN++
				r.Failures = append(r.Failures, fmt.Sprintf("%s: komitmen %s “%s” tidak ditemukan", c.ID, e.Who, e.Contains))
			}
		}
		for i, got := range out.Commitments {
			if !matched[i] {
				r.FP++
				r.Failures = append(r.Failures, fmt.Sprintf("%s: positif palsu komitmen %s “%s”", c.ID, got.Who, got.Text))
			}
		}
		sigs := map[string]bool{}
		for _, s := range out.Signals {
			sigs[s.Type] = true
		}
		for _, e := range c.Expect.Signals {
			if sigs[e] {
				r.TP++
				delete(sigs, e)
			} else {
				r.FN++
				r.Failures = append(r.Failures, fmt.Sprintf("%s: sinyal %s tidak ditemukan", c.ID, e))
			}
		}
		for typ := range sigs {
			r.FP++
			r.Failures = append(r.Failures, fmt.Sprintf("%s: positif palsu sinyal %s", c.ID, typ))
		}
		tmatched := map[int]bool{}
		for _, e := range c.Expect.Tasks {
			ok := false
			for i, t := range out.Tasks {
				if !tmatched[i] && strings.Contains(strings.ToLower(t.Text), strings.ToLower(e.Contains)) && (e.Assignee == "" || strings.Contains(t.AssigneeHint, e.Assignee)) {
					tmatched[i], ok = true, true
					break
				}
			}
			if ok {
				r.TP++
			} else {
				r.FN++
				r.Failures = append(r.Failures, fmt.Sprintf("%s: tugas “%s” tidak ditemukan", c.ID, e.Contains))
			}
		}
		for i, t := range out.Tasks {
			if !tmatched[i] {
				r.FP++
				r.Failures = append(r.Failures, fmt.Sprintf("%s: positif palsu tugas “%s”", c.ID, t.Text))
			}
		}
	}
	if r.TP+r.FP > 0 {
		r.Precision = float64(r.TP) / float64(r.TP+r.FP)
	}
	if r.TP+r.FN > 0 {
		r.Recall = float64(r.TP) / float64(r.TP+r.FN)
	}
	return r
}

// RunEval evaluates capture and writes docs/eval/capture-<provider>.md.
func (a *Agents) RunEval(ctx context.Context, root string) (string, error) {
	cases, err := LoadEvalCases(root)
	if err != nil {
		return "", err
	}
	r := a.EvaluateCapture(ctx, cases)
	var b strings.Builder
	fmt.Fprintf(&b, "# Eval Capture · provider %s · %s\n\n", r.Provider, time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "- Kasus: %d\n- TP %d · FP %d · FN %d\n- Precision: %.1f%%\n- Recall: %.1f%%\n\n", r.Cases, r.TP, r.FP, r.FN, r.Precision*100, r.Recall*100)
	if len(r.Failures) > 0 {
		b.WriteString("## Gagal (negatif & positif palsu)\n")
		for _, f := range r.Failures {
			b.WriteString("- " + f + "\n")
		}
	}
	out := b.String()
	_ = os.MkdirAll(filepath.Join(root, "docs", "eval"), 0o755)
	err = os.WriteFile(filepath.Join(root, "docs", "eval", "capture-"+r.Provider+".md"), []byte(out), 0o644)
	return out, err
}
