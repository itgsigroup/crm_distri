// Package seed loads tests/fixtures (extracted from the approved mockup plus
// seed_extra.json) into PostgreSQL. Every insert is keyed by a deterministic id
// and uses ON CONFLICT, so running the seed twice adds no rows.
package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"arc/packages/core/domain"
)

type kv = map[string]any

// Deal mirrors one entry of DEALS in the mockup.
type Deal struct {
	ID           string   `json:"id"`
	OdooStage    string   `json:"odooStage"`
	Signal       string   `json:"signal"`
	Closing      string   `json:"closing"`
	Tags         []string `json:"tags"`
	Prio         int      `json:"prio"`
	Activity     string   `json:"activity"`
	Account      string   `json:"account"`
	Sector       string   `json:"sector"`
	Branch       string   `json:"branch"`
	Owner        string   `json:"owner"`
	Opp          string   `json:"opp"`
	Value        float64  `json:"value"`
	Health       int      `json:"health"`
	Trend        int      `json:"trend"`
	Last         string   `json:"last"`
	LastVia      string   `json:"lastVia"`
	StageWhy     string   `json:"stageWhy"`
	ManualProb   int      `json:"manualProb"`
	Memo         string   `json:"memo"`
	MemoProv     []string `json:"memoProv"`
	Stakeholders []struct {
		N    string `json:"n"`
		Role string `json:"role"`
		Tag  string `json:"tag"`
		S    int    `json:"s"`
		Note string `json:"note"`
	} `json:"stakeholders"`
	Breakdown [][2]any `json:"breakdown"`
	Flags     []struct {
		T   string `json:"t"`
		S   string `json:"s"`
		K   string `json:"k"`
		Act string `json:"act"`
	} `json:"flags"`
	Commits struct {
		Kami   []Commit `json:"kami"`
		Mereka []Commit `json:"mereka"`
	} `json:"commits"`
	Timeline []struct {
		D   string `json:"d"`
		Via string `json:"via"`
		Who string `json:"who"`
		T   string `json:"t"`
		X   string `json:"x"`
		Hot bool   `json:"hot"`
	} `json:"timeline"`
}

// Commit is a ledger row in the mockup.
type Commit struct {
	T  string `json:"t"`
	S  string `json:"s"`
	St string `json:"st"`
	D  string `json:"d"`
}

// FixtureAction mirrors ACTIONS entries.
type FixtureAction struct {
	T       string   `json:"t"`
	Btn     string   `json:"btn"`
	Icon    string   `json:"icon"`
	Due     string   `json:"due"`
	Why     string   `json:"why"`
	Prep    string   `json:"prep"`
	Preview string   `json:"preview"`
	Steps   []string `json:"steps"`
}

// L2CRow mirrors L2C entries.
type L2CRow struct {
	Acc   string  `json:"acc"`
	Proj  string  `json:"proj"`
	SO    string  `json:"so"`
	Value float64 `json:"value"`
	Stage int     `json:"stage"`
	Days  int     `json:"days"`
	Bench int     `json:"bench"`
	Owner string  `json:"owner"`
	Note  string  `json:"note"`
	Aid   string  `json:"aid"`
	K     string  `json:"k"`
}

// Installed mirrors INSTALLED entries.
type Installed struct {
	Inst []struct {
		S           string `json:"s"`
		Y           string `json:"y"`
		W           string `json:"w"`
		C           string `json:"c"`
		WarrantyEnd string `json:"warranty_end"`
	} `json:"inst"`
	WS []struct {
		P   string  `json:"p"`
		St  string  `json:"st"`
		V   float64 `json:"v"`
		Why string  `json:"why"`
	} `json:"ws"`
}

// Contact mirrors CONTACTS.
type Contact struct {
	ID       string  `json:"id"`
	N        string  `json:"n"`
	Role     string  `json:"role"`
	Acc      *string `json:"acc"`
	Decision bool    `json:"decision"`
}

// Sales mirrors SALES.
type Sales struct {
	ID     string `json:"id"`
	N      string `json:"n"`
	Branch string `json:"branch"`
	No     string `json:"no"`
}

// ChatFixture mirrors CHATS.
type ChatFixture struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Sub    string `json:"sub"`
	Via    string `json:"via"`
	Acc    string `json:"acc"`
	Time   string `json:"time"`
	Unread int    `json:"unread"`
	Tag    struct {
		K string `json:"k"`
		T string `json:"t"`
	} `json:"tag"`
	Sug     []string `json:"sug"`
	Private bool     `json:"private"`
	Project *struct {
		ID    string `json:"id"`
		Stage string `json:"stage"`
		Next  string `json:"next"`
	} `json:"project"`
	Members []struct {
		N   string `json:"n"`
		R   string `json:"r"`
		Int bool   `json:"int"`
	} `json:"members"`
	Summary []string `json:"summary"`
	Tasks   []struct {
		T   string `json:"t"`
		Who string `json:"who"`
		Src string `json:"src"`
	} `json:"tasks"`
	Msgs []struct {
		D   string `json:"d"`
		F   string `json:"f"`
		Who string `json:"who"`
		Int bool   `json:"int"`
		T   string `json:"t"`
		Tm  string `json:"tm"`
		Ann *struct {
			K   string `json:"k"`
			T   string `json:"t"`
			Act string `json:"act"`
		} `json:"ann"`
	} `json:"msgs"`
}

// InboundFixture mirrors INBOUND.
type InboundFixture struct {
	ID     string `json:"id"`
	No     string `json:"no"`
	Via    string `json:"via"`
	When   string `json:"when"`
	First  string `json:"first"`
	Status string `json:"status"`
	Score  *int   `json:"score"`
	Ident  struct {
		Name    string `json:"name"`
		Role    string `json:"role"`
		Company string `json:"company"`
		Sources []struct {
			S string `json:"s"`
			C string `json:"c"`
			V string `json:"v"`
		} `json:"sources"`
	} `json:"ident"`
	Overview  string `json:"overview"`
	Solutions []struct {
		T   string  `json:"t"`
		V   float64 `json:"v"`
		K   string  `json:"k"`
		Why string  `json:"why"`
	} `json:"solutions"`
	Questions []struct {
		Q string `json:"q"`
		U string `json:"u"`
	} `json:"questions"`
}

// Fixtures is everything under tests/fixtures.
type Fixtures struct {
	Deals []Deal
	Won   []struct {
		Aid     string   `json:"aid"`
		Account string   `json:"account"`
		Opp     string   `json:"opp"`
		Value   float64  `json:"value"`
		Owner   string   `json:"owner"`
		Won     string   `json:"won"`
		Tags    []string `json:"tags"`
	}
	Leads []struct {
		Aid        string   `json:"aid"`
		Account    string   `json:"account"`
		Opp        string   `json:"opp"`
		Value      float64  `json:"value"`
		Owner      string   `json:"owner"`
		Closing    string   `json:"closing"`
		Tags       []string `json:"tags"`
		Prio       int      `json:"prio"`
		ManualProb int      `json:"manualProb"`
		Note       string   `json:"note"`
	}
	Actions   map[string]FixtureAction
	L2C       []L2CRow
	Installed map[string]Installed
	Sales     []Sales
	Contacts  []Contact
	Edges     struct {
		Months []string `json:"months"`
		Edges  [][3]any `json:"edges"`
	}
	Internal []struct {
		N      string `json:"n"`
		No     string `json:"no"`
		Unit   string `json:"unit"`
		Branch string `json:"branch"`
		Src    string `json:"src"`
	}
	Suspects []struct {
		No  string `json:"no"`
		N   string `json:"n"`
		Why string `json:"why"`
	}
	Chats     []ChatFixture
	Inbound   []InboundFixture
	AskCanned map[string]string
	AskSugg   map[string][]string
	WAHistory map[string]struct {
		Msg int    `json:"msg"`
		Com int    `json:"com"`
		Nw  int    `json:"nw"`
		T   string `json:"t"`
	}
	Screens map[string][2]string
	Extra   Extra
}

// Extra is seed_extra.json (partially typed; the rest is read as maps).
type Extra struct {
	MockupNow string `json:"mockup_now"`
	Users     []struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Email    string   `json:"email"`
		Role     string   `json:"role"`
		Branch   string   `json:"branch"`
		Initials string   `json:"initials"`
		WA       []string `json:"wa"`
	} `json:"users"`
	SalesUser   map[string]string `json:"sales_user"`
	AccountMeta map[string]struct {
		Rhythm int    `json:"rhythm"`
		Gov    bool   `json:"gov"`
		LastAt string `json:"last_at"`
		Source string `json:"source"`
	} `json:"account_meta"`
	AccountsExtra []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Sector string `json:"sector"`
		Branch string `json:"branch"`
		Owner  string `json:"owner"`
		Gov    bool   `json:"gov"`
	} `json:"accounts_extra"`
	ContactAccounts map[string]string `json:"contact_accounts"`
	Won             map[string]struct {
		Account string `json:"account"`
		WonAt   string `json:"won_at"`
		Source  string `json:"source"`
		LeadAt  string `json:"lead_at"`
	} `json:"won"`
	Lead map[string]struct {
		Account  string `json:"account"`
		LeadAt   string `json:"lead_at"`
		Source   string `json:"source"`
		Deadline string `json:"deadline"`
	} `json:"lead"`
	DealDeadlines    map[string]string `json:"deal_deadlines"`
	DealLeadAt       map[string]string `json:"deal_lead_at"`
	TodayCommitments map[string]struct {
		Text       string `json:"text"`
		Detail     string `json:"detail"`
		Status     string `json:"status"`
		Due        string `json:"due"`
		DraftReady bool   `json:"draft_ready"`
	} `json:"today_commitments"`
	SignalsToday []struct {
		Key         string  `json:"key"`
		Type        string  `json:"type"`
		Severity    string  `json:"severity"`
		Account     *string `json:"account"`
		Opportunity *string `json:"opportunity"`
		Title       string  `json:"title"`
		TodayDetail string  `json:"today_detail"`
		DetectedAt  string  `json:"detected_at"`
	} `json:"signals_today"`
	FlagTypes     map[string]string            `json:"flag_types"`
	ActionSummary map[string]string            `json:"action_summary"`
	ActionTypes   map[string]string            `json:"action_types"`
	ActionPayload map[string]map[string]string `json:"action_payload"`
	Queue         map[string]struct {
		Kind        string       `json:"kind"`
		ButtonLabel string       `json:"button_label"`
		Title       string       `json:"title"`
		Summary     string       `json:"summary"`
		PreviewFrom string       `json:"preview_from"`
		Preview     string       `json:"preview"`
		ContextNote string       `json:"context_note"`
		ResultText  string       `json:"result_text"`
		Toast       string       `json:"toast"`
		Tags        []domainPill `json:"tags"`
	} `json:"queue"`
	ExtraActions []struct {
		ID          string       `json:"id"`
		Agent       string       `json:"agent"`
		Type        string       `json:"type"`
		Kind        string       `json:"kind"`
		Account     string       `json:"account"`
		Opportunity *string      `json:"opportunity"`
		Title       string       `json:"title"`
		Icon        string       `json:"icon"`
		ButtonLabel string       `json:"button_label"`
		Due         string       `json:"due"`
		Summary     string       `json:"summary"`
		Why         string       `json:"why"`
		Prep        string       `json:"prep"`
		ContextNote string       `json:"context_note"`
		Impact      []kv         `json:"impact"`
		Steps       []string     `json:"steps"`
		Options     []kv         `json:"options"`
		Tags        []domainPill `json:"tags"`
		Confidence  float64      `json:"confidence"`
		ProposedBy  string       `json:"proposed_by"`
		CreatedAt   string       `json:"created_at"`
		Payload     kv           `json:"payload"`
	} `json:"extra_actions"`
	CreditProfiles []struct {
		Account     string  `json:"account"`
		Limit       float64 `json:"limit"`
		Open        float64 `json:"open"`
		AvgDays     int     `json:"avg_days"`
		OverdueNote string  `json:"overdue_note"`
		Phone       string  `json:"phone"`
	} `json:"credit_profiles"`
	Calendar []struct {
		ID        string       `json:"id"`
		Title     string       `json:"title"`
		Start     string       `json:"start"`
		Duration  int          `json:"duration"`
		Location  string       `json:"location"`
		Attendees string       `json:"attendees"`
		Account   string       `json:"account"`
		Internal  bool         `json:"internal"`
		Owner     string       `json:"owner"`
		Prep      string       `json:"prep"`
		Pills     []domainPill `json:"pills"`
	} `json:"calendar"`
	Invoices []struct {
		ID          string  `json:"id"`
		Number      string  `json:"number"`
		SO          string  `json:"so"`
		Account     string  `json:"account"`
		Amount      float64 `json:"amount"`
		InvoiceDate string  `json:"invoice_date"`
		Due         string  `json:"due"`
		Paid        string  `json:"paid"`
		Pattern     string  `json:"pattern"`
		Gov         bool    `json:"gov"`
		SPM         string  `json:"spm"`
		Label       string  `json:"label"`
		Category    string  `json:"category"`
	} `json:"invoices"`
	CashForecastItems []struct {
		ID       string  `json:"id"`
		Kind     string  `json:"kind"`
		Invoice  string  `json:"invoice"`
		Trigger  string  `json:"trigger"`
		Label    string  `json:"label"`
		Account  string  `json:"account"`
		Amount   float64 `json:"amount"`
		CashItem string  `json:"cash_item"`
		Detail   string  `json:"detail"`
	} `json:"cash_forecast_items"`
	PaymentHistory []struct {
		Account string   `json:"account"`
		Refs    [][2]any `json:"refs"`
	} `json:"payment_history"`
	L2CMeta map[string]struct {
		ID        string `json:"id"`
		Account   string `json:"account"`
		ProjectID string `json:"project_id"`
		Invoice   string `json:"invoice"`
	} `json:"l2c_meta"`
	InstalledExtra map[string]Installed `json:"installed_extra"`
	WarrantyEnds   map[string]string    `json:"warranty_ends"`
	Tenders        []struct {
		ID            string   `json:"id"`
		Title         string   `json:"title"`
		Agency        string   `json:"agency"`
		Source        string   `json:"source"`
		HPS           float64  `json:"hps"`
		Deadline      string   `json:"deadline"`
		Description   string   `json:"description"`
		Keywords      []string `json:"keywords"`
		Reasons       string   `json:"reasons"`
		ResearchScore int      `json:"research_score"`
	} `json:"tenders"`
	TenderKeywords []string `json:"tender_keywords"`
	WASessions     []struct {
		ID           string `json:"id"`
		User         string `json:"user"`
		Label        string `json:"label"`
		Status       string `json:"status"`
		LastEventMin int    `json:"last_event_min"`
		QRSeconds    int    `json:"qr_seconds"`
		Messages30d  int    `json:"messages_30d"`
	} `json:"wa_sessions"`
	Groups []struct {
		ID      string `json:"id"`
		JID     string `json:"jid"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Read    bool   `json:"read"`
		Session string `json:"session"`
		Account string `json:"account"`
		Members int    `json:"members"`
	} `json:"groups"`
	UnlistedGroups int `json:"unlisted_groups"`
	PrivacyRules   []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Detail  string `json:"detail"`
		Enabled bool   `json:"enabled"`
		Locked  bool   `json:"locked"`
	} `json:"privacy_rules"`
	Connectors []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Sub   string `json:"sub"`
		Logo  string `json:"logo"`
		Color string `json:"color"`
		Text  string `json:"text"`
		Grp   string `json:"grp"`
	} `json:"connectors"`
	AIClients []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Logo  string `json:"logo"`
		Color string `json:"color"`
		Desc  string `json:"desc"`
		Users int    `json:"users"`
		Last  string `json:"last"`
	} `json:"ai_clients"`
	APIKeys []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Prefix string   `json:"prefix"`
		Scopes []string `json:"scopes"`
		Last   string   `json:"last"`
		RPM    int      `json:"rpm"`
		Calls  int      `json:"calls"`
	} `json:"api_keys"`
	Calibration []struct {
		Agent    string `json:"agent"`
		Approved int    `json:"approved"`
		Total    int    `json:"total"`
	} `json:"calibration"`
	LearnedRules []struct {
		Agent   string `json:"agent"`
		Pattern string `json:"pattern"`
		Text    string `json:"text"`
	} `json:"learned_rules"`
	Coaching  map[string]string `json:"coaching"`
	TeamStats map[string]struct {
		ResponseMin    int `json:"response_min"`
		FollowupOnTime int `json:"followup_on_time"`
	} `json:"team_stats"`
	AgentFeed []struct {
		Agent string `json:"agent"`
		HTML  string `json:"html"`
	} `json:"agent_feed"`
	TodayActivity  map[string]int `json:"today_activity"`
	Policies       map[string]any `json:"policies"`
	CommitAccuracy []struct {
		Q         string  `json:"q"`
		Committed float64 `json:"committed"`
		Actual    float64 `json:"actual"`
	} `json:"commit_accuracy"`
	History struct {
		Sources           map[string][2]int `json:"sources"`
		Customers         int               `json:"customers"`
		Referrers         int               `json:"referrers"`
		ExpansionAccounts int               `json:"expansion_accounts"`
		L2CAfter          []int             `json:"lead_to_cash_after"`
		L2CBefore         []int             `json:"lead_to_cash_before"`
		W2IAfter          []int             `json:"won_to_invoice_after"`
		W2IBefore         []int             `json:"won_to_invoice_before"`
		DSOAfter          []int             `json:"dso_after"`
		DSOBefore         []int             `json:"dso_before"`
		RespAfter         []int             `json:"response_min_after"`
		RespBefore        []int             `json:"response_min_before"`
		W2BRecent         []int             `json:"won_to_bast_recent"`
	} `json:"history"`
	Funnel struct {
		Month    string         `json:"month"`
		Counts   map[string]int `json:"counts"`
		Channels []string       `json:"channels"`
	} `json:"funnel"`
	FunnelRaw       map[string]any            `json:"-"`
	BriefCounts     map[string]int            `json:"brief_counts"`
	Phones          map[string]string         `json:"phones"`
	Emails          map[string]string         `json:"emails"`
	ChatThreads     map[string]map[string]any `json:"chat_threads"`
	ChatDays        map[string]string         `json:"chat_days"`
	InternalFull    map[string]string         `json:"internal_numbers_full"`
	SuspectsFull    map[string]string         `json:"suspects_full"`
	InboundFull     map[string]string         `json:"inbound_full"`
	InboundReceived map[string]string         `json:"inbound_received"`
	AskSeed         []string                  `json:"ask_seed"`
	AskExtra        map[string]struct {
		Paragraphs []string `json:"paragraphs"`
		Followup   string   `json:"followup"`
	} `json:"ask_extra"`
}

type domainPill struct {
	K    string `json:"k"`
	T    string `json:"t"`
	Icon string `json:"icon,omitempty"`
}

// FixtureDir returns tests/fixtures under the repository root.
func FixtureDir(root string) string { return filepath.Join(root, "tests", "fixtures") }

// Load reads every fixture file.
func Load(dir string) (*Fixtures, error) {
	f := &Fixtures{}
	files := map[string]any{
		"deals.json": &f.Deals, "won.json": &f.Won, "leads.json": &f.Leads, "actions.json": &f.Actions,
		"l2c.json": &f.L2C, "installed.json": &f.Installed, "sales.json": &f.Sales, "contacts.json": &f.Contacts,
		"edges_m.json": &f.Edges, "internal.json": &f.Internal, "suspects.json": &f.Suspects, "chats.json": &f.Chats,
		"inbound.json": &f.Inbound, "ask_canned.json": &f.AskCanned, "ask_suggestions.json": &f.AskSugg,
		"wa_history.json": &f.WAHistory, "screens.json": &f.Screens, "seed_extra.json": &f.Extra,
	}
	for name, dst := range files {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return f, nil
}

// Clock converts mockup dates into ARC time: anchor + (date − mockup_now).
type Clock struct {
	MockupNow time.Time
	Anchor    time.Time
}

// Shift maps a mockup timestamp to ARC time.
func (c Clock) Shift(t time.Time) time.Time { return c.Anchor.Add(t.Sub(c.MockupNow)) }

// At parses "2026-09-26" or "2026-09-26T10:00" (Jakarta) and shifts it.
func (c Clock) At(s string) time.Time {
	if s == "" {
		return c.Anchor
	}
	layouts := []string{"2006-01-02T15:04", "2006-01-02"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, domain.Jakarta); err == nil {
			if l == "2006-01-02" {
				t = t.Add(10 * time.Hour)
			}
			return c.Shift(t)
		}
	}
	return c.Anchor
}

// AtPtr is At for optional values.
func (c Clock) AtPtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t := c.At(s)
	return &t
}

var monthIdx = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "mei": 5, "jun": 6, "jul": 7, "agu": 8, "sep": 9, "okt": 10, "nov": 11, "des": 12}
var reShortDate = regexp.MustCompile(`(?i)\b(\d{1,2})\s+(Jan|Feb|Mar|Apr|Mei|Jun|Jul|Agu|Sep|Okt|Nov|Des)\b`)

// ShortDate parses "26 Sep" (mockup year) and shifts it.
func (c Clock) ShortDate(s string, hour int) (time.Time, bool) {
	m := reShortDate.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false
	}
	day, _ := strconv.Atoi(m[1])
	mon := monthIdx[strings.ToLower(m[2])]
	year := c.MockupNow.Year()
	if mon < int(c.MockupNow.Month())-6 {
		year++
	}
	t := time.Date(year, time.Month(mon), day, hour, 0, 0, 0, domain.Jakarta)
	return c.Shift(t), true
}

// MaskPhone renders "+62 812-••••-4471".
func MaskPhone(p string) string {
	n := domain.NormalizePhone(p)
	if len(n) < 9 {
		return p
	}
	return fmt.Sprintf("+62 %s-••••-%s", n[2:5], n[len(n)-4:])
}
