// Package access is the role master (Pengaturan → Pengguna & peran): a role narrows its base system role — the
// screens of its menu (also enforced on the matching API routes), the proposal kinds its users decide, and whether
// they hold a WhatsApp number. A role never grants more than its base (ADR 0019).
package access

import (
	"slices"
	"strings"

	"distri-arc/internal/domain"
	"distri-arc/internal/proposals"
)

// Bases are the system roles every server check uses.
var Bases = []string{"ceo", "admin", "finance", "sales", "warehouse"}

// BaseLabel names the base roles.
var BaseLabel = map[string]string{"ceo": "CEO", "admin": "Admin", "finance": "Finance", "sales": "Sales", "warehouse": "Gudang"}

// Screen is one menu entry.
type Screen struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Screens in menu order.
var Screens = []Screen{
	{"today", "Pusat kendali"}, {"orch", "Orchestrator"}, {"chat", "Chat WhatsApp"}, {"orbit", "Orbit"}, {"kuad", "Segmen"},
	{"net", "Peta relasi"}, {"dealer", "Dealer"}, {"stock", "Push stok"}, {"ar", "Kredit · kas"}, {"users", "Pengguna & peran"}, {"conn", "Pengaturan"}, {"konsep", "Panduan"},
}

// KindLabels name the proposal kinds a role may decide.
var KindLabels = map[string]string{
	domain.KindFollowup: "Follow-up dealer", domain.KindCollect: "Pengingat penagihan", domain.KindInstallment: "Skema cicilan",
	domain.KindCreditRelease: "Rilis kredit di atas limit", domain.KindCreditLimit: "Perubahan limit", domain.KindCreditHold: "Tahan kredit",
	domain.KindPriceCounter: "Harga khusus", domain.KindPushStock: "Bundle / push stok", domain.KindSODraft: "SO draft",
	domain.KindReturn: "Retur", domain.KindTransfer: "Transfer stok", domain.KindPORequest: "Permintaan PO", domain.KindNewDealer: "Dealer baru",
	domain.KindPriceList: "Kirim daftar harga", domain.KindReply: "Balasan chat", domain.KindPlanChange: "Perubahan rencana",
}

// BaseScreens is the menu a base role may have (Pengaturan only for CEO and admin; finance works on credit,
// warehouse on stock).
func BaseScreens(base string) []string {
	switch base {
	case "sales":
		return []string{"today", "orch", "chat", "orbit", "kuad", "net", "dealer", "stock", "konsep"}
	case "finance":
		return []string{"today", "orch", "orbit", "kuad", "dealer", "ar", "konsep"}
	case "warehouse":
		return []string{"today", "chat", "stock", "dealer", "konsep"}
	}
	return []string{"today", "orch", "chat", "orbit", "kuad", "net", "dealer", "stock", "ar", "users", "conn", "konsep"}
}

// BaseKinds are the proposal kinds a base role may decide.
func BaseKinds(base string) []string {
	var out []string
	for _, k := range domain.AllKinds {
		if slices.Contains(proposals.RolesFor(k), base) {
			out = append(out, k)
		}
	}
	return out
}

// Narrow keeps the chosen items the base allows (nil choice = everything the base allows). The CEO is never
// narrowed: someone must always reach every screen and decision.
func Narrow(base string, allowed, chosen []string) []string {
	if chosen == nil || base == "ceo" {
		return slices.Clone(allowed)
	}
	out := []string{}
	for _, a := range allowed {
		if slices.Contains(chosen, a) {
			out = append(out, a)
		}
	}
	return out
}

// Role is a user's resolved access.
type Role struct {
	Key       string
	Name      string
	Base      string
	Screens   []string // effective
	Decide    []string // effective
	WAAllowed bool
}

// Resolve computes the effective access of a role row (screens/decide nil = all of the base).
func Resolve(key, name, base string, screens, decide []string, wa bool) Role {
	if key == "" { // an account without a role row (older seed / CLI): its base role, unnarrowed
		key, name = base, BaseLabel[base]
	}
	if name == "" {
		name = key
	}
	return Role{Key: key, Name: name, Base: base, Screens: Narrow(base, BaseScreens(base), screens), Decide: Narrow(base, BaseKinds(base), decide), WAAllowed: wa || base == "ceo"}
}

// guarded maps API path prefixes to the screen they belong to. Shared reads (dealers, proposals, /stock/push and
// /dealers/credit-tight used on Pusat kendali) stay open to every role.
var guarded = []struct{ prefix, screen string }{
	{"/api/chat/", "chat"}, {"/api/wa/groups", "chat"}, {"/api/wa/pair", "chat"},
	{"/api/credit/", "ar"},
	{"/api/roles", "users"},
	{"/api/relasi", "net"},
	{"/api/stock/aging", "stock"}, {"/api/stock/critical", "stock"}, {"/api/stock/sales-by-product", "stock"},
}

// ScreenForPath is the screen an API path needs, or "" when any signed-in user may call it.
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
