// Package domain holds ARC's core types and enums. It must not import
// transport or storage packages.
package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Evidence points a reasoning claim back to its source.
type Evidence struct {
	InteractionID int64  `json:"interaction_id,omitempty"`
	DocumentID    string `json:"document_id,omitempty"`
	Source        string `json:"source,omitempty"`
	Quote         string `json:"quote,omitempty"`
	At            string `json:"at,omitempty"`
}

// Provenance must accompany every AI-produced fact. Without it the fact is not stored.
type Provenance struct {
	Evidence      []Evidence `json:"evidence"`
	Confidence    float64    `json:"confidence"`
	Model         string     `json:"model"`
	PromptVersion string     `json:"prompt_version"`
}

// Valid reports whether the provenance is complete enough to persist.
func (p Provenance) Valid() bool {
	return len(p.Evidence) > 0 && p.Confidence > 0 && p.Model != ""
}

// Roles.
const (
	RoleCEO     = "ceo"
	RoleManager = "manager"
	RoleSales   = "sales"
	RoleFinance = "finance"
	RoleOps     = "ops"
)

// Scopes for API keys and tokens. ScopeHuman is reserved for user sessions.
const (
	ScopeRead    = "read"
	ScopePropose = "propose"
	ScopeEvents  = "events"
	ScopeSignals = "signals"
	ScopeHuman   = "human"
)

// Action statuses.
const (
	ActionProposed  = "proposed"
	ActionApproved  = "approved"
	ActionEdited    = "edited"
	ActionRejected  = "rejected"
	ActionSnoozed   = "snoozed"
	ActionExecuted  = "executed"
	ActionCancelled = "cancelled"
)

// RejectReasons is the fixed list a human must pick from when rejecting.
var RejectReasons = []string{
	"Tidak tepat waktu",
	"Salah kontak / jalur",
	"Sudah dilakukan",
	"Tidak sesuai kebijakan",
	"Konteks agen kurang",
}

// Agents.
const (
	AgentCapture     = "Capture agent"
	AgentHygiene     = "Hygiene agent"
	AgentResearch    = "Research agent"
	AgentFollowUp    = "Follow-up agent"
	AgentMeetingPrep = "Meeting prep agent"
	AgentForecast    = "Forecast agent"
	AgentCollection  = "Collection agent"
	AgentIdentity    = "Identity agent"
	AgentBrief       = "Chief agent"
	AgentPricing     = "Pricing agent"
	AgentCredit      = "Credit guardrail"
)

// HealthComponents in display order with their Indonesian labels.
var HealthComponents = []struct{ Key, Label string }{
	{"engagement", "Engagement"},
	{"multithreading", "Multi-threading"},
	{"momentum", "Momentum"},
	{"fit", "Kecocokan solusi"},
	{"sentiment", "Sentimen"},
}

// StakeholderLabels maps tags to UI labels.
var StakeholderLabels = map[string]string{
	"decision":    "Pengambil keputusan",
	"champion":    "Champion",
	"influencer":  "Pemengaruh",
	"user":        "Pengguna",
	"procurement": "Pengadaan",
	"ghost":       "Mutasi",
	"former":      "Mantan",
}

// L2CStages are the five lead-to-cash segments shown on the Kas screen, plus lunas.
var L2CStages = []string{"persiapan", "pemasangan", "bast", "invoice", "menunggu_bayar", "lunas"}

// L2CStageLabels maps stage keys to labels.
var L2CStageLabels = map[string]string{
	"persiapan": "Persiapan", "pemasangan": "Pemasangan", "bast": "BAST",
	"invoice": "Invoice", "menunggu_bayar": "Menunggu bayar", "lunas": "Lunas",
}

// L2CIndex returns the 1-based stage index (lunas = 6).
func L2CIndex(stage string) int {
	for i, s := range L2CStages {
		if s == stage {
			return i + 1
		}
	}
	return 0
}

// Band classifies a 0–100 score into the health band.
func Band(h int) string {
	switch {
	case h >= 70:
		return "good"
	case h >= 50:
		return "warn"
	default:
		return "bad"
	}
}

// FormatRp mirrors the UI formatter: billions as "Rp 2,4 M", millions as "Rp 412 jt".
func FormatRp(v float64) string {
	if v >= 1e9 {
		x := v / 1e9
		var s string
		if math.Mod(v, 1e9) == 0 {
			s = fmt.Sprintf("%.1f", x)
		} else {
			s = fmt.Sprintf("%.2f", x)
		}
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		return "Rp " + strings.ReplaceAll(s, ".", ",") + " M"
	}
	return fmt.Sprintf("Rp %d jt", int(math.Round(v/1e6)))
}

// FormatRp1 renders an aggregate with one decimal in billions ("Rp 11,9 M").
func FormatRp1(v float64) string {
	if v >= 1e9 || v == 0 {
		s := fmt.Sprintf("%.1f", v/1e9)
		if v == 0 {
			return "Rp 0"
		}
		return "Rp " + strings.ReplaceAll(s, ".", ",") + " M"
	}
	return FormatRp(v)
}

// Initials builds a two-letter avatar label, skipping honorifics.
func Initials(name string) string {
	n := name
	for _, p := range []string{"Pak ", "Bu ", "dr. "} {
		n = strings.TrimPrefix(n, p)
	}
	var out []rune
	for _, w := range strings.Fields(n) {
		r := []rune(w)
		if len(r) > 0 {
			out = append(out, r[0])
		}
		if len(out) == 2 {
			break
		}
	}
	return strings.ToUpper(string(out))
}

// Jakarta is the business timezone for schedules and day boundaries.
var Jakarta = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}()

var monthsID = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
var monthsLongID = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
var daysID = []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// ShortDate renders "26 Sep".
func ShortDate(t time.Time) string {
	t = t.In(Jakarta)
	return fmt.Sprintf("%d %s", t.Day(), monthsID[t.Month()-1])
}

// LongDate renders "Senin, 28 September 2026".
func LongDate(t time.Time) string {
	t = t.In(Jakarta)
	return fmt.Sprintf("%s, %d %s %d", daysID[t.Weekday()], t.Day(), monthsLongID[t.Month()-1], t.Year())
}

// DayLabel renders "Rabu, 24 Sep".
func DayLabel(t time.Time) string {
	t = t.In(Jakarta)
	return fmt.Sprintf("%s, %d %s", daysID[t.Weekday()], t.Day(), monthsID[t.Month()-1])
}

// ClockID renders "16.20".
func ClockID(t time.Time) string {
	t = t.In(Jakarta)
	return fmt.Sprintf("%02d.%02d", t.Hour(), t.Minute())
}

// MonthName returns the short Indonesian month name.
func MonthName(m time.Month) string { return monthsID[m-1] }

// MonthLong returns the long Indonesian month name.
func MonthLong(m time.Month) string { return monthsLongID[m-1] }

// DaysBetween counts whole calendar days from a to b in Jakarta time.
func DaysBetween(a, b time.Time) int {
	da := time.Date(a.In(Jakarta).Year(), a.In(Jakarta).Month(), a.In(Jakarta).Day(), 0, 0, 0, 0, Jakarta)
	db := time.Date(b.In(Jakarta).Year(), b.In(Jakarta).Month(), b.In(Jakarta).Day(), 0, 0, 0, 0, Jakarta)
	return int(math.Round(db.Sub(da).Hours() / 24))
}

// RelativeDay renders "hari ini", "kemarin" or "N hari lalu".
func RelativeDay(t, now time.Time) string {
	d := DaysBetween(t, now)
	switch {
	case d <= 0:
		return "hari ini"
	case d == 1:
		return "kemarin"
	default:
		return fmt.Sprintf("%d hari lalu", d)
	}
}

// Quarter returns the quarter number and its last day.
func Quarter(t time.Time) (int, time.Time) {
	t = t.In(Jakarta)
	q := (int(t.Month())-1)/3 + 1
	end := time.Date(t.Year(), time.Month(q*3)+1, 1, 0, 0, 0, 0, Jakarta).AddDate(0, 0, -1)
	return q, end
}

// QuarterStart returns the first day of t's quarter.
func QuarterStart(t time.Time) time.Time {
	t = t.In(Jakarta)
	q := (int(t.Month()) - 1) / 3
	return time.Date(t.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, Jakarta)
}

// WorkdaysUntil counts Mon–Fri days after `from` up to and including `to`.
func WorkdaysUntil(from, to time.Time) int {
	n := 0
	f := from.In(Jakarta)
	end := to.In(Jakarta)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, Jakarta)
	for d := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, Jakarta).AddDate(0, 0, 1); !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n++
		}
	}
	return n
}

// NormalizePhone reduces Indonesian numbers to the 62xxxxxxxx form.
func NormalizePhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	switch {
	case strings.HasPrefix(s, "62"):
		return s
	case strings.HasPrefix(s, "0"):
		return "62" + s[1:]
	case strings.HasPrefix(s, "8"):
		return "62" + s
	}
	return s
}

// FormatM1 always renders billions with one decimal ("Rp 0,7 M").
func FormatM1(v float64) string {
	if v == 0 {
		return "Rp 0"
	}
	return "Rp " + strings.ReplaceAll(fmt.Sprintf("%.1f", v/1e9), ".", ",") + " M"
}
