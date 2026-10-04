package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"arc/packages/connectors/odoo"
	"arc/packages/core/domain"
)

// SyncCommitmentActivities mirrors open "kami" commitments as Odoo activities on
// the linked crm.lead and marks them done once ARC sees the commitment fulfilled
// (Stage 10). Internal Odoo records only — nothing reaches a customer. Every
// write is whitelisted, carries the "via ARC" marker, and lands in odoo_writes + audit.
func (a *App) SyncCommitmentActivities(ctx context.Context) (map[string]int, error) {
	out := map[string]int{"created": 0, "done": 0, "failed": 0}

	// 1. New open commitments → mail.activity create.
	rows, err := a.DB.Pool.Query(ctx, `SELECT c.id, c.text, c.detail, c.due_at, c.evidence::text, o.source_id
		FROM commitments c
		JOIN LATERAL (
			SELECT op.source_id FROM opportunities op
			WHERE op.source_system='odoo' AND COALESCE(op.source_id,'')<>''
			  AND (op.id = c.opportunity_id OR (c.opportunity_id IS NULL AND op.account_id = c.account_id AND op.status='open'))
			ORDER BY (op.id = c.opportunity_id) DESC, op.expected_revenue DESC LIMIT 1) o ON true
		WHERE c.who='kami' AND c.status IN ('open','late') AND c.odoo_activity_id IS NULL`)
	if err != nil {
		return out, err
	}
	type pending struct {
		id, text, detail, evidence, leadID string
		due                                *time.Time
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.text, &p.detail, &p.due, &p.evidence, &p.leadID); err == nil {
			todo = append(todo, p)
		}
	}
	rows.Close()
	modelID := a.odooModelID(ctx, "crm.lead")
	for _, p := range todo {
		var lead int64
		if _, err := fmt.Sscanf(p.leadID, "%d", &lead); err != nil || lead == 0 {
			continue
		}
		deadline := domain.Now()
		if p.due != nil {
			deadline = *p.due
		}
		note := strings.TrimSpace(p.detail)
		if q := firstEvidenceQuote(p.evidence); q != "" {
			note = strings.TrimSpace(note + "\n“" + q + "”")
		}
		vals := map[string]any{"res_model": "crm.lead", "res_id": lead, "summary": trunc(p.text, 120),
			"note": strings.TrimSpace(note + "\n" + odoo.Marker("komitmen "+p.id)), "date_deadline": deadline.In(domain.Jakarta).Format("2006-01-02")}
		if modelID > 0 {
			vals["res_model_id"] = modelID
		}
		op := odoo.WriteOp{Model: "mail.activity", Method: "create", Values: vals}
		actID, err := a.OdooWriter.Apply(ctx, op)
		a.recordOdooWrite(ctx, "", op, err)
		if err != nil {
			out["failed"]++
			continue
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE commitments SET odoo_activity_id=$2, updated_at=now() WHERE id=$1`, p.id, actID)
		out["created"]++
	}

	// 2. Fulfilled commitments → mark their activity done (once).
	rows, err = a.DB.Pool.Query(ctx, `SELECT id, odoo_activity_id FROM commitments WHERE status='done' AND odoo_activity_id IS NOT NULL AND NOT odoo_activity_done`)
	if err != nil {
		return out, err
	}
	type done struct {
		id  string
		act int64
	}
	var finished []done
	for rows.Next() {
		var d done
		if err := rows.Scan(&d.id, &d.act); err == nil {
			finished = append(finished, d)
		}
	}
	rows.Close()
	for _, d := range finished {
		op := odoo.WriteOp{Model: "mail.activity", Method: "action_done", ID: d.act}
		_, err := a.OdooWriter.Apply(ctx, op)
		a.recordOdooWrite(ctx, "", op, err)
		if err != nil {
			out["failed"]++
			continue
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE commitments SET odoo_activity_done=true, updated_at=now() WHERE id=$1`, d.id)
		out["done"]++
	}
	return out, nil
}

// odooModelID resolves ir.model id (required by mail.activity in real Odoo); 0 when unknown (FakeOdoo).
func (a *App) odooModelID(ctx context.Context, model string) int64 {
	recs, err := a.OdooReader.SearchRead(ctx, "ir.model", []any{[]any{"model", "=", model}}, []string{"id"}, odoo.SearchOpts{Limit: 1})
	if err != nil || len(recs) == 0 {
		return 0
	}
	switch v := recs[0]["id"].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

func firstEvidenceQuote(raw string) string {
	var ev []domain.Evidence
	if json.Unmarshal([]byte(raw), &ev) != nil {
		return ""
	}
	for _, e := range ev {
		if strings.TrimSpace(e.Quote) != "" {
			return trunc(e.Quote, 200)
		}
	}
	return ""
}
