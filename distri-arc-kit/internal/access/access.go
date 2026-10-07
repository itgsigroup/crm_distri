// Package access is the role master (sidebar → Peran): a role is fully custom — the pages it opens (also enforced on
// their API routes), its data scope (all data / only its own dealers and chats), the proposal kinds it decides,
// policy rights (CEO level) and whether its users hold a WhatsApp number. From these a base is derived (ceo / sales /
// admin / finance) and stored in users.role, which the server's data-scope checks read (ADR 0020).
package access

import (
	"slices"
	"strings"

	"distri-arc/internal/domain"
	"distri-arc/internal/proposals"
)

// Screen is one page of the app.
type Screen struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Group   string `json:"group"`
	AllData bool   `json:"all_data"` // only for roles that see all data (management pages, credit across dealers)
}

// Screens in menu order.
var Screens = []Screen{
	{"today", "Pusat kendali", "Kendali", false}, {"orch", "Orchestrator", "Kendali", false}, {"chat", "Chat WhatsApp", "Kendali", false},
	{"orbit", "Orbit", "Dealer", false}, {"kuad", "Segmen", "Dealer", false}, {"net", "Peta relasi", "Dealer", false}, {"dealer", "Dealer", "Dealer", false},
	{"stock", "Push stok", "Operasi", false}, {"ar", "Kredit · kas", "Operasi", true},
	{"users", "Pengguna", "Master data", true}, {"roles", "Peran & akses", "Master data", true}, {"branches", "Cabang", "Master data", true},
	{"mcp", "MCP Claude", "Sistem", true}, {"conn", "Pengaturan", "Sistem", true}, {"konsep", "Panduan", "Sistem", false},
}

// Kind is a proposal kind a role may decide.
type Kind struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Policies bool   `json:"policies"` // needs policy rights (CEO level)
}

// KindLabels name the proposal kinds.
var KindLabels = map[string]string{
	domain.KindFollowup: "Follow-up dealer", domain.KindCollect: "Pengingat penagihan", domain.KindInstallment: "Skema cicilan",
	domain.KindCreditRelease: "Rilis kredit di atas limit", domain.KindCreditLimit: "Perubahan limit", domain.KindCreditHold: "Tahan kredit",
	domain.KindPriceCounter: "Harga khusus", domain.KindPushStock: "Bundle / push stok", domain.KindSODraft: "SO draft",
	domain.KindReturn: "Retur", domain.KindTransfer: "Transfer stok", domain.KindPORequest: "Permintaan PO", domain.KindNewDealer: "Dealer baru",
	domain.KindPriceList: "Kirim daftar harga", domain.KindReply: "Balasan chat", domain.KindPlanChange: "Perubahan rencana",
}

// Kinds lists every decidable kind; releasing credit above the limit stays with policy holders (CEO approval).
func Kinds() []Kind {
	out := make([]Kind, 0, len(domain.AllKinds))
	for _, k := range domain.AllKinds {
		out = append(out, Kind{Key: k, Label: KindLabels[k], Policies: k == domain.KindCreditRelease})
	}
	return out
}

// Role is a role's access, normalised.
type Role struct {
	Key       string
	Name      string
	Base      string   // derived: ceo | sales | admin | finance
	Screens   []string // pages
	Decide    []string // proposal kinds
	Scope     string   // all | own
	Policies  bool
	WAAllowed bool
}

// Normalize keeps known pages and kinds and applies the safety rules: policy rights imply all data; a role limited
// to its own data cannot open pages that show everyone's data; only policy holders release credit above the limit.
func Normalize(r Role) Role {
	if r.Policies {
		r.Scope = "all"
	}
	if r.Scope != "own" {
		r.Scope = "all"
	}
	screens := []string{}
	for _, s := range Screens {
		if slices.Contains(r.Screens, s.Key) && (!s.AllData || r.Scope != "own") {
			screens = append(screens, s.Key)
		}
	}
	decide := []string{}
	for _, k := range Kinds() {
		if slices.Contains(r.Decide, k.Key) && (!k.Policies || r.Policies) {
			decide = append(decide, k.Key)
		}
	}
	r.Screens, r.Decide = screens, decide
	r.Base = DeriveBase(r)
	return r
}

// DeriveBase is the base the server's data checks use: policy rights → ceo; own data → sales; management pages
// (Pengguna, Pengaturan) → admin; otherwise finance (all data, no management).
func DeriveBase(r Role) string {
	switch {
	case r.Policies:
		return "ceo"
	case r.Scope == "own":
		return "sales"
	case slices.Contains(r.Screens, "users") || slices.Contains(r.Screens, "conn"):
		return "admin"
	}
	return "finance"
}

// LegacyScreens is the menu of an account without a role row (older seed / CLI), by its base.
func LegacyScreens(base string) []string {
	switch base {
	case "sales":
		return []string{"today", "orch", "chat", "orbit", "kuad", "net", "dealer", "stock", "konsep"}
	case "finance":
		return []string{"today", "orch", "orbit", "kuad", "dealer", "ar", "konsep"}
	case "warehouse":
		return []string{"today", "chat", "stock", "dealer", "konsep"}
	}
	return []string{"today", "orch", "chat", "orbit", "kuad", "net", "dealer", "stock", "ar", "users", "roles", "branches", "mcp", "conn", "konsep"}
}

// LegacyKinds is what an account without a role row decides, by its base.
func LegacyKinds(base string) []string {
	var out []string
	for _, k := range domain.AllKinds {
		if slices.Contains(proposals.RolesFor(k), base) {
			out = append(out, k)
		}
	}
	return out
}

// Resolve computes a user's access from their role row; without one (key "") the legacy access of their base.
func Resolve(key, name, base string, screens, decide []string, scope string, policies, wa bool) Role {
	if key == "" {
		return Role{Key: base, Name: BaseLabel[base], Base: base, Screens: LegacyScreens(base), Decide: LegacyKinds(base),
			Scope: map[bool]string{true: "own", false: "all"}[base == "sales"], Policies: base == "ceo", WAAllowed: true}
	}
	r := Normalize(Role{Key: key, Name: name, Screens: screens, Decide: decide, Scope: scope, Policies: policies, WAAllowed: wa})
	if name == "" {
		r.Name = key
	}
	return r
}

// BaseLabel names the derived bases.
var BaseLabel = map[string]string{"ceo": "CEO", "admin": "Admin", "finance": "Finance", "sales": "Sales", "warehouse": "Gudang"}

// guarded maps API path prefixes to the page they belong to. Shared reads (dealers, proposals, /stock/push and
// /dealers/credit-tight used on Pusat kendali, the branch list) stay open to every signed-in user.
var guarded = []struct{ prefix, screen string }{
	{"/api/chat/", "chat"}, {"/api/wa/groups", "chat"}, {"/api/wa/pair", "chat"}, {"/api/wa/links", "chat"},
	{"/api/credit/", "ar"},
	{"/api/relasi", "net"},
	{"/api/stock/aging", "stock"}, {"/api/stock/critical", "stock"}, {"/api/stock/sales-by-product", "stock"},
	{"/api/users", "users"},
	{"/api/roles", "roles"},
	{"/api/mcp/", "mcp"},
}

// ScreenForPath is the page an API path needs, or "" when any signed-in user may call it.
func ScreenForPath(path string) string {
	for _, g := range guarded {
		if strings.HasPrefix(path, g.prefix) {
			return g.screen
		}
	}
	return ""
}

// ValidKey reports whether s can be a role key (lower case, digits, dash).
func ValidKey(s string) bool {
	if len(s) < 2 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
