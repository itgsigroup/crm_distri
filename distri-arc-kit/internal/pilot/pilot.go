// Package pilot measures the branch pilot (stage 14): proposal acceptance and decision time per agent, order tepat
// jadwal, DSO, lewat jadwal caught before churn, the privacy and send audit, and when an agent may get its automatic
// steps back (confidence ≥ 80% two weeks in a row). Numbers come from the database and internal/metrics, never an LLM.
package pilot

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// AgentStat is one agent's line of the pilot dashboard.
type AgentStat struct {
	Agent        string `json:"agent"`
	Proposed     int64  `json:"proposed"`
	Approved     int64  `json:"approved"`
	Edited       int64  `json:"edited"`
	Rejected     int64  `json:"rejected"`
	Expired      int64  `json:"expired"`
	Open         int64  `json:"open"`
	Autonomous   int64  `json:"autonomous"`
	AcceptPct    *int   `json:"accept_pct"`          // (approved + edited) / human decisions
	MedianMin    *int   `json:"median_decision_min"` // proposal → human decision
	Weeks        []Conf `json:"weeks"`               // weekly confidence, oldest first
	Eligible     bool   `json:"eligible"`            // may get its automatic steps back
	Unlocked     bool   `json:"unlocked"`
	EligibleNote string `json:"eligible_note"`
}

// Conf is one week of human decisions for an agent.
type Conf struct {
	Week     time.Time `json:"week"`
	Accepted int64     `json:"accepted"`
	Rejected int64     `json:"rejected"`
	Pct      int       `json:"pct"`
}

// Check is one line of the privacy & send audit; Violations must be 0.
type Check struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Violations int64  `json:"violations"`
}

// Report is Pengaturan → Pilot for a period.
type Report struct {
	Mode         string      `json:"mode"`
	Branch       string      `json:"branch"`
	StartedAt    string      `json:"started_at"`
	ShadowUntil  string      `json:"shadow_until"`
	Day          int         `json:"day"` // day of the pilot (1-based), 0 before it starts
	From         time.Time   `json:"from"`
	To           time.Time   `json:"to"`
	Agents       []AgentStat `json:"agents"`
	OnSchedule   int         `json:"on_schedule_pct"`
	DSO          int         `json:"dso_days"`
	Targets      domain.KPITargets
	AtRisk       int64   `json:"at_risk"`
	Caught       int64   `json:"caught_before_churn"`
	Churned      int64   `json:"churned"`
	Audit        []Check `json:"audit"`
	AuditOK      bool    `json:"audit_ok"`
	Decisions    int64   `json:"decisions"`
	AcceptPct    *int    `json:"accept_pct"`
	SentInShadow int64   `json:"sent_in_shadow"`
}

// Service computes pilot reports.
type Service struct {
	St    *store.Store
	Clock clock.Clock
}

func pct(a, b int64) *int {
	if b == 0 {
		return nil
	}
	v := int(math.Round(float64(a) * 100 / float64(b)))
	return &v
}

// Week is the Monday (WIB) of t's ISO week.
func Week(t time.Time) time.Time {
	d := clock.Today(t)
	off := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -off)
}

// Quarter is the SOW confirmation period of t ("2026-Q4").
func Quarter(t time.Time) string {
	t = t.In(clock.WIB)
	return fmt.Sprintf("%d-Q%d", t.Year(), (int(t.Month())-1)/3+1)
}

func parseDay(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", s, clock.WIB)
	return t, err == nil
}

// Build computes the report for [from, to) in the pilot branch.
func (s Service) Build(ctx context.Context, from, to time.Time) (Report, error) {
	pol, err := policy.Load(ctx, s.St.Q)
	if err != nil {
		return Report{}, err
	}
	pp := pol.Pilot
	r := Report{Mode: pp.Mode, Branch: pp.Branch, StartedAt: pp.StartedAt, From: from, To: to, Targets: pol.KPI, Audit: []Check{}, Agents: []AgentStat{}}
	now := s.Clock.Now()
	if start, ok := parseDay(pp.StartedAt); ok {
		r.ShadowUntil = start.AddDate(0, 0, pp.ShadowDays).Format("2006-01-02")
		if !now.Before(start) {
			r.Day = int(clock.Today(now).Sub(start).Hours()/24) + 1
		}
	}
	stats, err := s.St.Q.PilotAgentStats(ctx, gen.PilotAgentStatsParams{Since: from, Until: to, Branch: pp.Branch})
	if err != nil {
		return r, err
	}
	by := map[string]gen.PilotAgentStatsRow{}
	for _, x := range stats {
		by[x.Agent] = x
	}
	weeks, err := s.St.Q.PilotWeeklyConfidence(ctx, gen.PilotWeeklyConfidenceParams{Since: from.AddDate(0, 0, -7*max(pp.UnlockWeeks, 1)), Branch: pp.Branch})
	if err != nil {
		return r, err
	}
	conf := map[string][]Conf{}
	for _, w := range weeks {
		c := Conf{Week: w.Week, Accepted: w.Accepted, Rejected: w.Rejected}
		if p := pct(w.Accepted, w.Accepted+w.Rejected); p != nil {
			c.Pct = *p
		}
		conf[w.Agent] = append(conf[w.Agent], c)
	}
	var acc, dec int64
	for _, name := range domain.AgentNames {
		x := by[name]
		a := AgentStat{Agent: name, Proposed: x.Proposed, Approved: x.Approved, Edited: x.Edited, Rejected: x.Rejected, Expired: x.Expired,
			Open: x.Open, Autonomous: x.Autonomous, Weeks: conf[name], Unlocked: slices.Contains(pp.Unlocked, name)}
		if a.Weeks == nil {
			a.Weeks = []Conf{}
		}
		a.AcceptPct = pct(x.Approved+x.Edited, x.Approved+x.Edited+x.Rejected)
		if x.Agent != "" && x.MedianDecisionMin >= 0 {
			m := int(math.Round(x.MedianDecisionMin))
			a.MedianMin = &m
		}
		a.Eligible, a.EligibleNote = eligible(a.Weeks, pp, Week(now))
		acc += x.Approved + x.Edited
		dec += x.Approved + x.Edited + x.Rejected
		r.Agents = append(r.Agents, a)
	}
	r.Decisions, r.AcceptPct = dec, pct(acc, dec)

	b, err := views.NewBuilder(s.St, s.Clock).Board(ctx)
	if err != nil {
		return r, err
	}
	k := b.KPI(pp.Branch, nil)
	r.OnSchedule, r.DSO = k.OnSchedulePct, k.DSODays
	d, err := s.St.Q.PilotDriftCaught(ctx, gen.PilotDriftCaughtParams{Since: from, Until: to, Branch: pp.Branch})
	if err != nil {
		return r, err
	}
	r.AtRisk, r.Caught, r.Churned = d.AtRisk, d.Caught, d.Churned
	if err := s.audit(ctx, &r, pp, from, to); err != nil {
		return r, err
	}
	return r, nil
}

// eligible applies the unlock rule: the last `unlock_weeks` complete weeks each have ≥ min decisions and
// confidence ≥ unlock_confidence.
func eligible(ws []Conf, pp domain.PilotPolicy, thisWeek time.Time) (bool, string) {
	need := max(pp.UnlockWeeks, 1)
	var done []Conf
	for _, w := range ws {
		if w.Week.Before(thisWeek) {
			done = append(done, w)
		}
	}
	if len(done) < need {
		return false, fmt.Sprintf("butuh %d minggu penuh keputusan (baru %d)", need, len(done))
	}
	last := done[len(done)-need:]
	for i, w := range last {
		if i > 0 && w.Week.Sub(last[i-1].Week) != 7*24*time.Hour {
			return false, "minggu tidak berturut-turut"
		}
		if n := w.Accepted + w.Rejected; n < int64(pp.MinDecisionsPerWeek) {
			return false, fmt.Sprintf("minggu %s hanya %d keputusan (< %d)", clock.DayMonth(w.Week), n, pp.MinDecisionsPerWeek)
		}
		if w.Pct < pp.UnlockConfidence {
			return false, fmt.Sprintf("minggu %s confidence %d%% (< %d%%)", clock.DayMonth(w.Week), w.Pct, pp.UnlockConfidence)
		}
	}
	return true, fmt.Sprintf("%d minggu berturut ≥ %d%%", need, pp.UnlockConfidence)
}

func (s Service) audit(ctx context.Context, r *Report, pp domain.PilotPolicy, from, to time.Time) error {
	q := s.St.Q
	add := func(key, label string, n int64, err error) error {
		if err != nil {
			return err
		}
		r.Audit = append(r.Audit, Check{Key: key, Label: label, Violations: n})
		return nil
	}
	n, err := q.AuditUnapprovedSends(ctx, gen.AuditUnapprovedSendsParams{Since: from, Until: to})
	if err := add("unapproved_sends", "Kirim ke dealer / Odoo tanpa persetujuan tercatat", n, err); err != nil {
		return err
	}
	if start, ok := parseDay(pp.StartedAt); ok {
		end := start.AddDate(0, 0, pp.ShadowDays)
		lo, hi := maxTime(from, start), minTime(to, end)
		var sent int64
		if lo.Before(hi) {
			if sent, err = q.AuditSentBetween(ctx, gen.AuditSentBetweenParams{Since: lo, Until: hi}); err != nil {
				return err
			}
		}
		r.SentInShadow = sent
		if err := add("shadow_sends", "Kirim selama mode bayangan", sent, nil); err != nil {
			return err
		}
	}
	n, err = q.AuditSystemToNonInternal(ctx, gen.AuditSystemToNonInternalParams{Since: from, Until: to})
	if err := add("system_to_dealer", "Alert sistem ke selain grup internal", n, err); err != nil {
		return err
	}
	n, err = q.AuditInternalDMs(ctx, gen.AuditInternalDMsParams{Since: from, Until: to})
	if err := add("internal_dm", "DM antar nomor internal tersimpan", n, err); err != nil {
		return err
	}
	n, err = q.AuditInternalGroupToDealer(ctx, gen.AuditInternalGroupToDealerParams{Since: from, Until: to})
	if err := add("internal_group_dealer", "Pesan grup internal masuk ke dealer", n, err); err != nil {
		return err
	}
	n, err = q.AuditOutboundIdentified(ctx, gen.AuditOutboundIdentifiedParams{Since: from, Until: to})
	if err := add("outbound_identified", "Identifikasi nomor yang tidak menghubungi lebih dulu", n, err); err != nil {
		return err
	}
	r.AuditOK = true
	for _, c := range r.Audit {
		if c.Violations > 0 {
			r.AuditOK = false
		}
	}
	return nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Period is the pilot so far: from its start (or 14 days back) to the end of today.
func (s Service) Period(ctx context.Context) (time.Time, time.Time, error) {
	pol, err := policy.Load(ctx, s.St.Q)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	today := clock.Today(s.Clock.Now())
	from := today.AddDate(0, 0, -13)
	if start, ok := parseDay(pol.Pilot.StartedAt); ok {
		from = start
	}
	return from, today.AddDate(0, 0, 1), nil
}

// Snapshot stores the report of the week containing t (job pilot.snapshot, idempotent per week and branch).
func (s Service) Snapshot(ctx context.Context, t time.Time) (Report, error) {
	from := Week(t)
	r, err := s.Build(ctx, from, from.AddDate(0, 0, 7))
	if err != nil {
		return r, err
	}
	b, _ := json.Marshal(r)
	return r, s.St.Q.UpsertPilotWeek(ctx, gen.UpsertPilotWeekParams{Week: from, Branch: r.Branch, Data: b})
}

// ErrNotEligible refuses opening an agent's automatic steps before the unlock rule holds.
var ErrNotEligible = errors.New("agen belum memenuhi syarat buka otonomi")

// Unlock opens an agent's automatic steps (CEO, pilot live). The rule is checked on the server.
func (s Service) Unlock(ctx context.Context, agent string, by *gen.GetUserByEmailRow) (domain.PilotPolicy, error) {
	pol, err := policy.Load(ctx, s.St.Q)
	if err != nil {
		return domain.PilotPolicy{}, err
	}
	from, to, err := s.Period(ctx)
	if err != nil {
		return pol.Pilot, err
	}
	r, err := s.Build(ctx, from, to)
	if err != nil {
		return pol.Pilot, err
	}
	for _, a := range r.Agents {
		if a.Agent != agent {
			continue
		}
		if !a.Eligible {
			return pol.Pilot, fmt.Errorf("%w: %s", ErrNotEligible, a.EligibleNote)
		}
		pp := pol.Pilot
		if !slices.Contains(pp.Unlocked, agent) {
			pp.Unlocked = append(pp.Unlocked, agent)
		}
		return pp, s.save(ctx, pp, by)
	}
	return pol.Pilot, fmt.Errorf("agen %q tidak dikenal", agent)
}

// SetMode switches the pilot (start → shadow with today's date; shadow → live; off).
func (s Service) SetMode(ctx context.Context, mode, branch string, by *gen.GetUserByEmailRow) (domain.PilotPolicy, error) {
	pol, err := policy.Load(ctx, s.St.Q)
	if err != nil {
		return domain.PilotPolicy{}, err
	}
	pp := pol.Pilot
	if branch != "" {
		pp.Branch = branch
	}
	if mode == "shadow" && (pp.Mode == "off" || pp.StartedAt == "") {
		pp.StartedAt = clock.Today(s.Clock.Now()).Format("2006-01-02")
		pp.Unlocked = []string{}
	}
	pp.Mode = mode
	if pp.Unlocked == nil {
		pp.Unlocked = []string{}
	}
	return pp, s.save(ctx, pp, by)
}

func (s Service) save(ctx context.Context, pp domain.PilotPolicy, by *gen.GetUserByEmailRow) error {
	b, _ := json.Marshal(pp)
	email, salesID := "arc ctl", (*uuid.UUID)(nil)
	if by != nil {
		email, salesID = deref(by.Email), by.SalesUserID
	}
	_, err := policy.Save(ctx, s.St, "pilot", b, salesID, email, s.Clock.Now())
	return err
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

// CSV renders a report as the weekly export (one line per agent, then the KPI lines).
func CSV(r Report) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	opt := func(p *int) string {
		if p == nil {
			return ""
		}
		return strconv.Itoa(*p)
	}
	_ = w.Write([]string{"periode", r.From.Format("2006-01-02") + " s.d. " + r.To.AddDate(0, 0, -1).Format("2006-01-02"), "cabang", r.Branch, "mode", r.Mode})
	_ = w.Write([]string{"agen", "saran", "disetujui", "diedit", "ditolak", "ditunda", "terbuka", "otonom", "persen_diterima", "median_menit_keputusan", "boleh_otonom"})
	for _, a := range r.Agents {
		_ = w.Write([]string{a.Agent, itoa(a.Proposed), itoa(a.Approved), itoa(a.Edited), itoa(a.Rejected), itoa(a.Expired), itoa(a.Open), itoa(a.Autonomous),
			opt(a.AcceptPct), opt(a.MedianMin), strconv.FormatBool(a.Eligible)})
	}
	_ = w.Write([]string{})
	_ = w.Write([]string{"indikator", "nilai", "target"})
	_ = w.Write([]string{"order_tepat_jadwal_pct", strconv.Itoa(r.OnSchedule), strconv.Itoa(r.Targets.OnSchedulePct)})
	_ = w.Write([]string{"dso_hari", strconv.Itoa(r.DSO), strconv.Itoa(r.Targets.DSODays)})
	_ = w.Write([]string{"lewat_jadwal", itoa(r.AtRisk), ""})
	_ = w.Write([]string{"tertangkap_sebelum_churn", itoa(r.Caught), ""})
	_ = w.Write([]string{"menjadi_churn", itoa(r.Churned), ""})
	for _, c := range r.Audit {
		_ = w.Write([]string{"audit_" + c.Key, itoa(c.Violations), "0"})
	}
	w.Flush()
	return buf.Bytes()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
