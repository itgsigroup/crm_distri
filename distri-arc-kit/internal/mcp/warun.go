package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"distri-arc/internal/events"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/store/gen"
)

// KeyWAAnalysis is the schedule of Analisa WhatsApp every 2 hours (ADR 0037).
const KeyWAAnalysis = "wa_analysis"

// recordWARun records Claude's Analisa WhatsApp for a whole-WhatsApp cycle (screen:wa or all) as a run of the
// external schedule — the claude.ai routine that drives it has no other way to report (ADR 0037 rev.). A refused
// submit is a failed run with the reason. Analisa lanjut on one chat or one dealer is not a run.
func (s *Server) recordWARun(ctx context.Context, c *Client, cycleID uuid.UUID, overview string, res orchestrator.SubmitResult, subErr error) {
	ctx = context.WithoutCancel(ctx)
	sch, err := s.St.Q.GetMCPScheduleByKey(ctx, ptr(KeyWAAnalysis))
	if err != nil || sch.Runner != "external" {
		return
	}
	cyc, err := s.St.Q.GetCycle(ctx, cycleID)
	if err != nil || (cyc.Scope != "all" && cyc.Scope != "screen:wa") {
		return
	}
	status, report, errText := "ok", strings.TrimSpace(overview), (*string)(nil)
	switch {
	case subErr != nil:
		status, errText = "error", ptr("Kiriman Claude ditolak: "+subErr.Error())
	case res.Accepted == 0 && report == "":
		status, errText = "error", ptr("Claude mengirim Analisa WhatsApp tanpa isi")
	}
	if report != "" {
		report = fmt.Sprintf("Siklus #%d · %d percakapan & pesan diterima\n\n%s", deref(cyc.Number), res.Accepted, report)
	}
	steps, _ := json.Marshal([]map[string]any{{"tool": "orchestrator_submit", "status": map[bool]string{true: "ok", false: "error"}[subErr == nil], "summary": fmt.Sprintf("%d diterima", res.Accepted)}})
	at := cyc.StartedAt
	if err := s.St.Q.RecordExternalRun(ctx, gen.RecordExternalRunParams{ScheduleID: sch.ID, Slot: &at, TriggeredBy: &c.Name, Status: status,
		Model: &c.Name, Report: nonEmpty(report), Steps: steps, Error: errText}); err != nil {
		s.Log.Warn("record wa run", "err", err)
		return
	}
	_ = events.Notify(ctx, s.St.Pool, "mcp_schedule", map[string]any{"schedule_id": sch.ID, "status": status})
}

func nonEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
