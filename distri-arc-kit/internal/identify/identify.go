// Package identify tells who an inbound unknown number is (AI Prospek, 05-agents › 6): WhatsApp Business profile
// (through the transport), Truecaller (adapter; fake without an API key), Getcontact (manual CSV import) and Odoo
// contacts. Privacy (09-policies-security): only numbers that wrote to a sales number first are identified.
package identify

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Source is one identification source as stored and shown in Chat → Nomor baru.
type Source struct {
	Source string `json:"source"` // wa_business | truecaller | getcontact | odoo
	Value  string `json:"value"`
	OK     string `json:"ok"` // "true" | "false" (kept as text: seed format)
	Name   string `json:"name,omitempty"`
}

// Result is the identification of a number.
type Result struct {
	WANumber  string   `json:"wa_number"`
	Sources   []Source `json:"sources"`
	BestName  string   `json:"best_name"`
	BestOrg   string   `json:"best_org"`
	Score     int      `json:"score"`
	Potential string   `json:"potential"`
}

// Truecaller looks a number up. Without TRUECALLER_API_KEY the Fake is used (OPEN-QUESTIONS: API access).
type Truecaller interface {
	Lookup(ctx context.Context, number string) (name string, ok bool, err error)
}

// FakeTruecaller answers from a fixed map (tests, demo).
type FakeTruecaller map[string]string

// Lookup implements Truecaller.
func (f FakeTruecaller) Lookup(_ context.Context, number string) (string, bool, error) {
	n, ok := f[wa.Digits(number)]
	return n, ok, nil
}

// ErrNotInbound refuses to identify a number that did not write to GSI first.
var ErrNotInbound = errors.New("nomor ini belum pernah mengirim pesan ke nomor sales — identifikasi hanya untuk nomor inbound")

// Service identifies numbers.
type Service struct {
	St         *store.Store
	Clock      clock.Clock
	Profiles   wa.ProfileReader // nil in the API process: the worker owns the WhatsApp connections
	Truecaller Truecaller
}

// Score combines sources: each named source counts 30, names that agree across sources add 14, an Odoo match is
// certain (100). Two agreeing sources (Getcontact + WA Business) give 74, like the sample "Toko Mandiri".
func Score(srcs []Source) int {
	var names []string
	for _, s := range srcs {
		if s.OK != "true" {
			continue
		}
		if s.Source == "odoo" {
			return 100
		}
		if s.Name != "" {
			names = append(names, s.Name)
		}
	}
	score := 30 * len(names)
	if agree(names) {
		score += 14
	}
	return min(score, 95)
}

// agree reports whether two names share a distinctive word ("Mandiri Elektronik Pati" ~ "Toko Mandiri – CCTV").
func agree(names []string) bool {
	stop := map[string]bool{"toko": true, "cv": true, "pt": true, "ud": true, "dan": true, "&": true, "–": true, "-": true, "cctv": false}
	seen := map[string]int{}
	for _, n := range names {
		words := map[string]bool{}
		for _, w := range strings.Fields(strings.ToLower(n)) {
			w = strings.Trim(w, "“”\"'.,()")
			if len(w) >= 4 && !stop[w] {
				words[w] = true
			}
		}
		for w := range words {
			seen[w]++
			if seen[w] >= 2 {
				return true
			}
		}
	}
	return false
}

// Identify gathers the sources for an inbound number, scores them and stores the result (identifications and the
// thread's context). It never runs for numbers that did not write first.
func (s *Service) Identify(ctx context.Context, number string) (Result, error) {
	num := wa.Digits(number)
	jid := wa.UserJID(num)
	th, err := s.St.Q.InboundNumber(ctx, &jid)
	if err != nil {
		return Result{}, ErrNotInbound
	}
	res := Result{WANumber: num}
	prev, _ := s.St.Q.GetIdentification(ctx, num)
	if prev.BestOrg != nil {
		res.BestOrg = *prev.BestOrg
	}
	// 1. WhatsApp Business profile (worker), else what an earlier run recorded
	if s.Profiles != nil && th.SalesWa != nil {
		if p, err := s.Profiles.Profile(ctx, *th.SalesWa, jid); err == nil && p.Business && p.Name != "" {
			v := fmt.Sprintf("Profil WA Business: “%s”", p.Name)
			if p.Category != "" {
				v += " · " + p.Category
			}
			res.Sources = append(res.Sources, Source{Source: "wa_business", Value: v, OK: "true", Name: p.Name})
		}
	}
	if !has(res.Sources, "wa_business") {
		var old []Source
		_ = json.Unmarshal(prev.Sources, &old)
		for _, o := range old {
			if o.Source == "wa_business" {
				if o.Name == "" {
					o.Name = quoted(o.Value)
				}
				res.Sources = append(res.Sources, o)
			}
		}
	}
	// 2. Truecaller
	if s.Truecaller != nil {
		if n, ok, err := s.Truecaller.Lookup(ctx, num); err == nil && ok {
			res.Sources = append(res.Sources, Source{Source: "truecaller", Value: fmt.Sprintf("Truecaller: “%s”", n), OK: "true", Name: n})
		}
	}
	// 3. Getcontact (manual CSV)
	gc, err := s.St.Q.GetcontactFor(ctx, num)
	if err != nil {
		return res, err
	}
	if len(gc) > 0 {
		res.Sources = append(res.Sources, Source{Source: "getcontact", Value: fmt.Sprintf("Getcontact: “%s” (%d tag)", gc[0].Name, gc[0].Tags), OK: "true", Name: gc[0].Name})
	}
	// 4. Odoo (contacts synced from res.partner)
	if c, err := s.St.Q.ContactByNumber(ctx, &num); err == nil {
		res.Sources = append(res.Sources, Source{Source: "odoo", Value: fmt.Sprintf("Odoo: %s · %s", deref(c.Name), c.DealerName), OK: "true", Name: c.DealerName})
	} else {
		res.Sources = append(res.Sources, Source{Source: "odoo", Value: "Belum ada di Odoo", OK: "false"})
	}
	res.Score = Score(res.Sources)
	res.BestName = bestName(res.Sources, deref(prev.BestName))
	res.Potential = deref(prev.Potential)
	if city, _, _ := strings.Cut(res.BestOrg, " · "); city != "" {
		if p, err := s.St.Q.CityPotential(ctx, city); err == nil && p.Dealers > 0 {
			res.Potential = fmt.Sprintf("%d dealer di %s, omzet median Rp %d jt/bulan. Tawarkan jadi dealer resmi dengan harga tier C dan ongkir subsidi untuk order ≥ Rp 5 jt.", p.Dealers, city, p.MedianOmzet/1_000_000)
		}
	}
	srcs, _ := json.Marshal(res.Sources)
	score := int16(res.Score)
	now := s.Clock.Now()
	if err := s.St.Q.SaveIdentification(ctx, gen.SaveIdentificationParams{WaNumber: num, Sources: srcs, BestName: &res.BestName, BestOrg: &res.BestOrg, Score: &score,
		Potential: strp(res.Potential), IdentifiedAt: &now}); err != nil {
		return res, err
	}
	ctxJSON, _ := json.Marshal(res)
	if err := s.St.Q.SetThreadIdentification(ctx, gen.SetThreadIdentificationParams{WaJid: &jid, Identification: ctxJSON}); err != nil {
		return res, err
	}
	return res, nil
}

func has(xs []Source, src string) bool {
	for _, x := range xs {
		if x.Source == src {
			return true
		}
	}
	return false
}

func quoted(v string) string {
	if i := strings.Index(v, "“"); i >= 0 {
		rest := v[i+len("“"):]
		if j := strings.Index(rest, "”"); j >= 0 {
			return rest[:j]
		}
	}
	return ""
}

// bestName prefers Odoo, then the WA Business name, then Truecaller, then Getcontact; an earlier curated name
// stays when the sources only confirm it.
func bestName(srcs []Source, prev string) string {
	for _, want := range []string{"odoo", "wa_business", "truecaller", "getcontact"} {
		for _, s := range srcs {
			if s.Source == want && s.OK == "true" && s.Name != "" {
				if prev != "" && agree([]string{prev, s.Name}) {
					return prev
				}
				return s.Name
			}
		}
	}
	return prev
}

// ImportResult summarises a Getcontact CSV import.
type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  []string `json:"skipped"`
}

// ImportGetcontact reads a manual Getcontact export: number,name[,tags]. Header optional; numbers normalised.
func ImportGetcontact(ctx context.Context, q *gen.Queries, r io.Reader, by *uuid.UUID, at time.Time) (ImportResult, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	var res ImportResult
	line := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, err
		}
		line++
		if len(rec) < 2 {
			res.Skipped = append(res.Skipped, fmt.Sprintf("baris %d: butuh nomor dan nama", line))
			continue
		}
		num := wa.Digits(rec[0])
		if line == 1 && len(num) < 8 {
			continue // header
		}
		if len(num) < 9 || len(num) > 15 {
			res.Skipped = append(res.Skipped, fmt.Sprintf("baris %d: nomor %q tidak valid", line, rec[0]))
			continue
		}
		name := strings.TrimSpace(rec[1])
		if name == "" {
			res.Skipped = append(res.Skipped, fmt.Sprintf("baris %d: nama kosong", line))
			continue
		}
		tags := 0
		if len(rec) > 2 {
			tags, _ = strconv.Atoi(strings.TrimSpace(rec[2]))
		}
		if err := q.UpsertGetcontact(ctx, gen.UpsertGetcontactParams{WaNumber: num, Name: name, Tags: int32(tags), ImportedBy: by, ImportedAt: at}); err != nil {
			return res, err
		}
		res.Imported++
	}
	return res, nil
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
