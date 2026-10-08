// Package domain holds the pure types of GSI Orbit (no database or HTTP). Names follow
// docs/design/01-glossary.md; JSON is snake_case, money in rupiah (int64), percentages 0–100.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Categories is the fixed product-mix list ("6 KAT"), in display order.
var Categories = [6]string{"Kamera & NVR", "HDD & storage", "Kabel & PoE", "Modul LED", "Fire alarm", "Aksesoris"}

// CategoryIndex returns the position of a KAT name, or -1.
func CategoryIndex(kat string) int {
	for i, c := range Categories {
		if c == kat {
			return i
		}
	}
	return -1
}

// Status (lingkar orbit).
const (
	StatusKeyAccount = "Key account"
	StatusAktif      = "Aktif"
	StatusAtRisk     = "At risk"
	StatusChurn      = "Churn"
	StatusBaru       = "Baru"
	StatusProspek    = "Prospek" // never ordered (ADR 0023): not drawn on the orbit, counted separately
)

// Activity (aktivitas relatif terhadap siklus order).
const (
	ActivityNormal   = "normal"
	ActivityMenurun  = "menurun"
	ActivityBerhenti = "berhenti"
	ActivityBaru     = "baru"
)

// Segments.
const (
	SegmentA       = "A"
	SegmentB       = "B"
	SegmentC       = "C"
	SegmentD       = "D"
	SegmentBaru    = "Baru"
	SegmentProspek = "Prospek"
)

// Credit states (sisa limit).
const (
	CreditAman      = "aman"
	CreditTipis     = "tipis"
	CreditOverLimit = "over limit"
	CreditOverdue   = "overdue"
	CreditCash      = "cash"
)

// Order-to-cash phases.
var Phases = [5]string{"Order", "Siap", "Kirim", "Invoice", "Bayar"}

// OrderLine is one sale.order.line.
type OrderLine struct {
	Product  string `json:"product"`
	Category string `json:"category"`
	Qty      int64  `json:"qty"`
	Price    int64  `json:"price"`
	Subtotal int64  `json:"subtotal"`
	Cost     int64  `json:"cost,omitempty"` // HPP per unit when the source has it (margin per product)
}

// Value returns the line value (subtotal, or qty × price when subtotal is missing).
func (l OrderLine) Value() int64 {
	if l.Subtotal != 0 {
		return l.Subtotal
	}
	return l.Qty * l.Price
}

// Order is a sale order (SO) with its order-to-cash timestamps.
type Order struct {
	ID          uuid.UUID   `json:"id"`
	Number      string      `json:"number"`
	State       string      `json:"state"` // order|siap|kirim|invoice|bayar|cancel
	OrderedAt   *time.Time  `json:"ordered_at"`
	ConfirmedAt *time.Time  `json:"confirmed_at"`
	ShippedAt   *time.Time  `json:"shipped_at"`
	InvoicedAt  *time.Time  `json:"invoiced_at"`
	PaidAt      *time.Time  `json:"paid_at"`
	Total       int64       `json:"total"`
	MarginPct   float64     `json:"margin_pct"`
	Lines       []OrderLine `json:"lines"`
	CreatedBy   string      `json:"created_by"`
}

// Invoice is a posted customer invoice (account.move).
type Invoice struct {
	ID       uuid.UUID  `json:"id"`
	OrderID  *uuid.UUID `json:"order_id"`
	Number   string     `json:"number"`
	IssuedAt time.Time  `json:"issued_at"`
	DueAt    time.Time  `json:"due_at"`
	Total    int64      `json:"total"`
	Paid     int64      `json:"paid"`
	PaidAt   *time.Time `json:"paid_at"`
	State    string     `json:"state"`
}

// Residual is the amount still open.
func (i Invoice) Residual() int64 { return i.Total - i.Paid }

// Contact is a person at a dealer.
type Contact struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	Role              string     `json:"role"`
	WANumber          string     `json:"wa_number"`
	IsPrimary         bool       `json:"is_primary"`
	LastInteractionAt *time.Time `json:"last_interaction_at"`
	Interactions90d   int        `json:"interactions_90d"`
}

// SOWEstimate is a share-of-wallet confirmation by sales.
type SOWEstimate struct {
	Quarter     string    `json:"quarter"`
	SOW         int       `json:"sow"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// DealerHistory is everything metrics.Compute needs about one dealer.
type DealerHistory struct {
	CreditLimit   int64
	TermsDays     int
	Orders        []Order
	Invoices      []Invoice
	Contacts      []Contact
	SOWEstimates  []SOWEstimate
	CompetitorSOW *int // AI Order extraction from WA (confidence ≥ 0.7), when available
	// OrderedBefore is the latest order older than the loaded history (ADR 0023): such a dealer is Churn, not Prospek
	OrderedBefore *time.Time
}

// Credit is the sisa-limit block of the metrics.
type Credit struct {
	Limit    int64    `json:"limit"`
	Exposure int64    `json:"exposure"`
	Room     *float64 `json:"room"` // nil for cash dealers
	State    string   `json:"state"`
	Late     bool     `json:"late"`
	LateDays int      `json:"late_days"` // oldest overdue invoice, days past due
	PayDays  int      `json:"pay_days"`
	OnTime   int      `json:"on_time"`
}

// ScoreParts are the five components of skor dealer (each 0–100).
type ScoreParts struct {
	Rhythm  int `json:"r"`
	SOW     int `json:"p"`
	Mix     int `json:"k"`
	Credit  int `json:"n"`
	Contact int `json:"i"`
}

// DealerMetrics is the computed state of a dealer (dealers.metrics_current).
type DealerMetrics struct {
	AsOf       time.Time  `json:"as_of"`
	Rhythm     *int       `json:"rhythm_days"`
	Last       *int       `json:"last_order_days"`
	Cyc        float64    `json:"cyc"`
	DueIn      *int       `json:"due_in"`
	Status     string     `json:"status"`
	Activity   string     `json:"activity"`
	Freq       *float64   `json:"freq"`
	AvgOrder   int64      `json:"avg_order"`
	OmzetBln   int64      `json:"omzet_bln"`
	Segment    string     `json:"segment"`
	SOW        int        `json:"sow"`
	SOWSource  string     `json:"sow_source"` // confirmed | estimated | default
	Mix        int        `json:"mix"`
	MixCats    [6]bool    `json:"mix_cats"`
	Credit     Credit     `json:"credit"`
	PICActive  int        `json:"pic_active"`
	Score      int        `json:"score"`
	ScoreParts ScoreParts `json:"score_parts"`
	Orders6m   int        `json:"orders_6m"`
	CycleDays  int        `json:"cycle_days"` // lama putaran: rata-rata hari Order → Bayar, 6 bulan
}

// ScoreBand maps a 0–100 score to good/warn/bad.
func ScoreBand(s int) string {
	switch {
	case s >= 70:
		return "good"
	case s >= 50:
		return "warn"
	default:
		return "bad"
	}
}

// StockItem is the stock of one SKU in one branch.
type StockItem struct {
	ID             uuid.UUID `json:"id"`
	Branch         string    `json:"branch"`
	SKU            string    `json:"sku"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	Qty            int       `json:"qty"`
	UnitCost       int64     `json:"unit_cost"`
	Value          int64     `json:"value"`
	AgeDays        int       `json:"age_days"`
	WeeklyVelocity float64   `json:"weekly_velocity"`
}

// DaysLeft estimates how many days the stock lasts at the current weekly velocity (nil when it does not move).
func (s StockItem) DaysLeft() *float64 {
	if s.WeeklyVelocity <= 0 {
		return nil
	}
	d := float64(s.Qty) / s.WeeklyVelocity * 7
	return &d
}
