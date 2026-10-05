// Package views builds the read models behind the REST API (Pusat kendali, Orbit, Segmen, Dealer, Push stok,
// Kredit · kas). Numbers come from internal/metrics; copy is composed by the web client (lib/i18n).
package views

import (
	"context"
	"encoding/json"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
	"distri-arc/internal/store"
)

// Owner is the sales who owns a dealer.
type Owner struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
	Branch   string `json:"branch"`
}

// Prev is the dealer's position three months ago (Segmen "titik putus").
type Prev struct {
	RhythmDays *int     `json:"rhythm_days"`
	AvgOrder   int64    `json:"avg_order"`
	Freq       *float64 `json:"freq"`
	Segment    string   `json:"segment"`
}

// BoardItem is one dealer as every board and list shows it.
type BoardItem struct {
	ID          string                 `json:"id"`
	UUID        uuid.UUID              `json:"uuid"`
	Name        string                 `json:"name"`
	ShortName   string                 `json:"short_name"`
	City        string                 `json:"city"`
	Branch      string                 `json:"branch"`
	Tier        string                 `json:"tier"`
	SegmentDesc string                 `json:"segment_desc"`
	Owner       Owner                  `json:"owner"`
	CreditLimit int64                  `json:"credit_limit"`
	Metrics     domain.DealerMetrics   `json:"metrics"`
	Composition []metrics.ProductShare `json:"composition"`
	Prev        *Prev                  `json:"prev"`
	RootCause   string                 `json:"root_cause,omitempty"`
}

// Board is a consistent picture of all dealers at one moment.
type Board struct {
	Today    time.Time
	Policies domain.PolicySet
	Items    []BoardItem
	Data     *dealersvc.Dataset
	byID     map[string]int
}

// Get returns a dealer by slug or uuid.
func (b *Board) Get(id string) (BoardItem, bool) {
	i, ok := b.byID[id]
	if !ok {
		return BoardItem{}, false
	}
	return b.Items[i], true
}

// Filter keeps the dealers of one sales (name or key, case-insensitive); empty keeps all.
func (b *Board) Filter(sales string) []BoardItem {
	if sales == "" || strings.EqualFold(sales, "all") {
		return append([]BoardItem(nil), b.Items...)
	}
	var out []BoardItem
	for _, it := range b.Items {
		if strings.EqualFold(it.Owner.Key, sales) || strings.EqualFold(it.Owner.Name, sales) {
			out = append(out, it)
		}
	}
	return out
}

// Builder assembles boards.
type Builder struct {
	st    *store.Store
	svc   *dealersvc.Service
	clock clock.Clock
}

// NewBuilder builds a read-model builder.
func NewBuilder(st *store.Store, c clock.Clock) *Builder {
	return &Builder{st: st, svc: dealersvc.New(st, c), clock: c}
}

// Service exposes the recompute service.
func (b *Builder) Service() *dealersvc.Service { return b.svc }

// Board loads every dealer with fresh metrics (computed from the same history the cache is built from).
func (b *Builder) Board(ctx context.Context) (*Board, error) {
	ds, err := b.svc.Load(ctx)
	if err != nil {
		return nil, err
	}
	prev, err := b.prev(ctx, ds)
	if err != nil {
		return nil, err
	}
	board := &Board{Today: ds.Today, Policies: ds.Policies, Data: ds, byID: map[string]int{}}
	for _, d := range ds.Dealers {
		h := ds.Histories[d.ID]
		m := metrics.Compute(h, ds.Policies, ds.Today)
		it := BoardItem{
			ID: deref(d.Slug), UUID: d.ID, Name: d.Name, ShortName: dealersvc.ShortName(d.Name), City: deref(d.City), Branch: d.Branch,
			Tier: deref(d.Tier), SegmentDesc: deref(d.SegmentDesc), CreditLimit: d.CreditLimit, Metrics: m,
			Composition: metrics.Composition(h.Orders, ds.Today, 4), Prev: prev[deref(d.Slug)],
		}
		if it.ID == "" {
			it.ID = d.ID.String()
		}
		if d.OwnerName != nil {
			it.Owner = Owner{Key: strings.ToLower(*d.OwnerName), Name: *d.OwnerName, Initials: Initials(*d.OwnerName), Branch: deref(d.OwnerBranch)}
		}
		if m.Rhythm != nil && m.Cyc > ds.Policies.Orbit.Drift {
			texts := []string{deref(d.Memo)}
			rows, err := b.st.Q.ListDealerSignalTexts(ctx, &d.ID)
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				texts = append(texts, deref(r))
			}
			it.RootCause = domain.RootCause(texts)
		}
		board.byID[it.ID] = len(board.Items)
		board.byID[d.ID.String()] = len(board.Items)
		board.Items = append(board.Items, it)
	}
	return board, nil
}

// prev returns the 3-months-ago position from the snapshot 90 days back, or db/seed/metrics_prev.json when
// no snapshot that old exists yet (docs/stages/01).
func (b *Builder) prev(ctx context.Context, ds *dealersvc.Dataset) (map[string]*Prev, error) {
	out := map[string]*Prev{}
	snaps, err := b.st.Q.ListSnapshotsOn(ctx, clock.Today(ds.Today).AddDate(0, 0, -90))
	if err != nil {
		return nil, err
	}
	slug := map[uuid.UUID]string{}
	for _, d := range ds.Dealers {
		slug[d.ID] = deref(d.Slug)
	}
	for _, s := range snaps {
		if s.AvgOrder == nil {
			continue
		}
		p := &Prev{AvgOrder: *s.AvgOrder, Freq: s.Freq, Segment: deref(s.Segment)}
		if s.RhythmDays != nil {
			r := int(*s.RhythmDays)
			p.RhythmDays = &r
		}
		out[slug[s.DealerID]] = p
	}
	if len(out) > 0 {
		return out, nil
	}
	raw, err := fs.ReadFile(db.Seed, "seed/metrics_prev.json")
	if err != nil {
		return out, nil
	}
	var seeded map[string]struct {
		RhythmDays int   `json:"rhythm_days"`
		AvgOrder   int64 `json:"avg_order"`
	}
	if err := json.Unmarshal(raw, &seeded); err != nil {
		return nil, err
	}
	for k, v := range seeded {
		r := v.RhythmDays
		p := &Prev{AvgOrder: v.AvgOrder}
		if r > 0 {
			p.RhythmDays = &r
		}
		p.Freq = metrics.Freq(p.RhythmDays)
		p.Segment = metrics.Segment(p.Freq, p.AvgOrder, ds.Policies)
		out[k] = p
	}
	return out, nil
}

// Initials returns the two-letter avatar of a sales: the first letter plus the next consonant for a single
// name (Andi → AN, Dewi → DW, Rizky → RZ, Fajar → FJ, as in the mockup), else the first letters of two words.
func Initials(name string) string {
	f := strings.Fields(name)
	if len(f) == 0 {
		return "?"
	}
	if len(f) > 1 {
		return strings.ToUpper(string([]rune(f[0])[:1]) + string([]rune(f[1])[:1]))
	}
	r := []rune(strings.ToUpper(f[0]))
	for _, c := range r[1:] {
		if !strings.ContainsRune("AEIOU", c) {
			return string(r[0]) + string(c)
		}
	}
	if len(r) > 1 {
		return string(r[:2])
	}
	return string(r)
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

// SortByCycDesc orders dealers furthest along their cycle first (dealer list).
func SortByCycDesc(items []BoardItem) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Metrics.Cyc > items[j].Metrics.Cyc })
}
