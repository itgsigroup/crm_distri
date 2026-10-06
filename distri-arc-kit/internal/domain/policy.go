package domain

// PolicySet is the typed view of the `policies` table (docs/design/09-policies-security.md). Every threshold
// used by internal/metrics comes from here; DefaultPolicies mirrors db/seed/policies.json.
type PolicySet struct {
	Orbit     OrbitPolicy            `json:"orbit.thresholds"`
	Segment   SegmentPolicy          `json:"segment.thresholds"`
	Credit    CreditPolicy           `json:"credit.rules"`
	Followup  FollowupPolicy         `json:"followup.rules"`
	Margin    MarginPolicy           `json:"margin.floor"`
	Stock     StockPolicy            `json:"stock.rules"`
	KPI       KPITargets             `json:"kpi.targets"`
	Autonomy  map[string]AutonomyRow `json:"autonomy.matrix"`
	Guard     AutonomyGuard          `json:"autonomy.guard"`
	OdooWrite OdooWritePolicy        `json:"odoo.write"`
	MCP       MCPPermissions         `json:"mcp.permissions"`
	LLM       LLMRouting             `json:"llm.routing"`
	Retention Retention              `json:"retention"`
	Pilot     PilotPolicy            `json:"pilot"`
}

type OrbitPolicy struct {
	Drift      float64 `json:"drift"`
	Churn      float64 `json:"churn"`
	KeyAccount struct {
		SOWMin    int `json:"sow_min"`
		OnTimeMin int `json:"on_time_min"`
	} `json:"key_account"`
}

type SegmentPolicy struct {
	FreqPerMonth        float64 `json:"freq_per_month"`
	SizeIDR             int64   `json:"size_idr"`
	NewDealerWaitOrders int     `json:"new_dealer_wait_orders"`
}

type CreditPolicy struct {
	RoomMin      float64          `json:"room_min"`
	PayMaxDays   int              `json:"pay_max_days"`
	DefaultLimit map[string]int64 `json:"default_limit"`
	LimitUp      struct {
		OnTimeMin      int `json:"on_time_min"`
		TightMonthsMin int `json:"tight_months_min"`
	} `json:"limit_up"`
	LimitDown struct {
		LateInvoices int `json:"late_invoices"`
		LateDays     int `json:"late_days"`
	} `json:"limit_down"`
	ReleaseOverLimit  string `json:"release_over_limit"`
	SOPSec001Required bool   `json:"sop_sec_001_required"`
}

type FollowupPolicy struct {
	GapDays           int    `json:"gap_days"`
	HMinus            int    `json:"h_minus"`
	MaxPerDayPerSales int    `json:"max_per_day_per_sales"`
	SecondFollowup    string `json:"second_followup"`
}

type MarginPolicy struct {
	Pct float64 `json:"pct"`
}

type StockPolicy struct {
	AgingDays            int     `json:"aging_days"`
	BundleMaxDiscountPct float64 `json:"bundle_max_discount_pct"`
	CriticalDays         float64 `json:"critical_days"`
}

type KPITargets struct {
	OnSchedulePct int `json:"on_schedule_pct"`
	DSODays       int `json:"dso_days"`
	StockTurnDays int `json:"stock_turn_days"`
}

// OdooWritePolicy: what Distri ARC writes to Odoo besides SO drafts (only when ODOO_WRITE=true).
type OdooWritePolicy struct {
	Notes bool `json:"notes"` // an internal note on the partner for every decided proposal
}

// AutonomyGuard narrows the matrix (04-orchestrator › Keputusan, ADR 0008). DealerMessages decides what an
// "auto" step that would message a dealer does: "confirm" (default) keeps it scheduled until a human presses
// "Jalankan sekarang"; "auto" lets the Orchestrator send it at its slot — only the owner may switch it on.
type AutonomyGuard struct {
	MinConfidence  float64 `json:"min_confidence"`
	DealerMessages string  `json:"dealer_messages"`
}

// AutoSendsMessages reports whether auto steps may message dealers without a human.
func (g AutonomyGuard) AutoSendsMessages() bool { return g.DealerMessages == "auto" }

// AutonomyRow is one agent's row of the autonomy matrix.
type AutonomyRow struct {
	Auto    []string          `json:"auto"`
	Approve []string          `json:"approve"`
	Never   []string          `json:"never"`
	Labels  map[string]string `json:"labels"`
}

type MCPPermissions struct {
	AllowReanalyze          bool `json:"allow_reanalyze"`
	AllowPlanUpdateProposal bool `json:"allow_plan_update_proposal"`
	AllowSend               bool `json:"allow_send"`
	MaskPIIInRead           bool `json:"mask_pii_in_read"`
	MaxCyclesPerHour        int  `json:"max_cycles_per_hour"`
}

type LLMRouting struct {
	Mode       string `json:"mode"` // api | mcp | both
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Fallback   string `json:"fallback"`
	BatchHours []int  `json:"batch_hours"`
	Timezone   string `json:"timezone"`
}

type Retention struct {
	ChatDays      int `json:"chat_days"`
	SignalsMonths int `json:"signals_months"`
	LLMCallsDays  int `json:"llm_calls_days"`
}

// PilotPolicy runs a branch pilot (stage 14). Shadow: the Orchestrator analyses and people decide, but nothing is
// executed automatically and nothing is sent (decisions only calibrate). Live: sends after human approval; automatic
// steps only for agents unlocked after two weeks of confidence ≥ unlock_confidence. Off: the autonomy matrix as is.
type PilotPolicy struct {
	Mode                string   `json:"mode"` // off | shadow | live
	Branch              string   `json:"branch"`
	StartedAt           string   `json:"started_at"` // YYYY-MM-DD, empty before the pilot
	ShadowDays          int      `json:"shadow_days"`
	UnlockConfidence    int      `json:"unlock_confidence"`
	UnlockWeeks         int      `json:"unlock_weeks"`
	MinDecisionsPerWeek int      `json:"min_decisions_per_week"`
	Unlocked            []string `json:"unlocked"` // agents whose automatic steps are open again
}

// Shadow reports whether nothing may leave Distri ARC (no sends, no Odoo writes, no automatic steps).
func (p PilotPolicy) Shadow() bool { return p.Mode == "shadow" }

// AutoAllowed reports whether an agent may take automatic steps under the pilot.
func (p PilotPolicy) AutoAllowed(agent string) bool {
	switch p.Mode {
	case "shadow":
		return false
	case "live":
		for _, a := range p.Unlocked {
			if a == agent {
				return true
			}
		}
		return false
	}
	return true
}

// DefaultPolicies returns the glossary defaults (used by unit tests and as a fallback for missing keys).
func DefaultPolicies() PolicySet {
	var p PolicySet
	p.Orbit.Drift, p.Orbit.Churn = 1.2, 2.0
	p.Orbit.KeyAccount.SOWMin, p.Orbit.KeyAccount.OnTimeMin = 50, 85
	p.Segment = SegmentPolicy{FreqPerMonth: 1.5, SizeIDR: 20_000_000, NewDealerWaitOrders: 2}
	p.Credit.RoomMin, p.Credit.PayMaxDays = 0.40, 35
	p.Credit.DefaultLimit = map[string]int64{"A": 250_000_000, "B": 150_000_000, "C": 0, "new": 25_000_000}
	p.Credit.LimitUp.OnTimeMin, p.Credit.LimitUp.TightMonthsMin = 90, 3
	p.Credit.LimitDown.LateInvoices, p.Credit.LimitDown.LateDays = 2, 14
	p.Credit.ReleaseOverLimit, p.Credit.SOPSec001Required = "ceo_approve", true
	p.Followup = FollowupPolicy{GapDays: 14, HMinus: 1, MaxPerDayPerSales: 12, SecondFollowup: "approve"}
	p.Margin.Pct = 9
	p.Stock = StockPolicy{AgingDays: 90, BundleMaxDiscountPct: 8, CriticalDays: 10}
	p.KPI = KPITargets{OnSchedulePct: 85, DSODays: 30, StockTurnDays: 40}
	p.OdooWrite = OdooWritePolicy{Notes: true}
	p.Guard = AutonomyGuard{MinConfidence: 0.8, DealerMessages: "confirm"}
	p.MCP = MCPPermissions{AllowReanalyze: true, AllowPlanUpdateProposal: true, MaskPIIInRead: true, MaxCyclesPerHour: 6}
	p.LLM = LLMRouting{Mode: "both", Provider: "anthropic", Model: "claude-sonnet-5-5", Fallback: "openai:gpt-4.1", BatchHours: []int{6, 20}, Timezone: "Asia/Jakarta"}
	p.Retention = Retention{ChatDays: 90, SignalsMonths: 24, LLMCallsDays: 180}
	p.Pilot = PilotPolicy{Mode: "off", Branch: "Semarang", ShadowDays: 14, UnlockConfidence: 80, UnlockWeeks: 2, MinDecisionsPerWeek: 5, Unlocked: []string{}}
	return p
}
