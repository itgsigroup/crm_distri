package seed

import (
	"fmt"
	"time"
)

// history generates the closed deals of the last 24 months (plus older "legacy"
// wins) that power win rate by source, Denyut bisnis, flywheel and win/loss
// patterns. Targets come from seed_extra.json → history; generation is fully
// deterministic.
func (s *Seeder) history() error {
	h := s.F.Extra.History
	arcStart := s.C.At("2026-07-01")
	now := s.C.Anchor

	type acct struct {
		id, name string
		gov      bool
	}
	synthetic := func(prefix string, n int, govEvery int) []acct {
		out := make([]acct, n)
		for i := range out {
			out[i] = acct{id: fmt.Sprintf("%s%02d", prefix, i+1), name: fmt.Sprintf("Pelanggan historis %s%02d", prefix[len(prefix)-1:], i+1), gov: govEvery > 0 && i%govEvery == 0}
		}
		return out
	}
	histAccts := synthetic("hist-a", 11, 3)
	legacyAccts := synthetic("hist-l", 15, 4)
	for _, a := range append(append([]acct{}, histAccts...), legacyAccts...) {
		sector := "Enterprise · Umum"
		if a.gov {
			sector = "Pemerintah · Umum"
		}
		if err := s.exec(`INSERT INTO accounts(id,name,sector,branch,is_government,created_at) VALUES ($1,$2,$3,'Semarang',$4,$5) ON CONFLICT (id) DO NOTHING`,
			a.id, a.name, sector, a.gov, now.AddDate(-3, 0, 0)); err != nil {
			return err
		}
	}
	govOf := map[string]bool{"magelang": true, "sleman": true, "rsud": true}
	for _, a := range histAccts {
		govOf[a.id] = a.gov
	}

	type deal struct {
		id, account, source, line, status string
		stakeholders                      int
		silent                            bool
		referrer                          string
		lead, quotation, won, lost        *time.Time
		invoice, bast, paid               *time.Time
		response                          *int
	}
	var deals []deal
	t := func(x time.Time) *time.Time { return &x }

	// Account assignment of the historical wins.
	ekspansiAccts := []string{"amarta", "semen", "bsd", "panti", "baja", "sleman", "magelang"}
	otherWinAccts := []string{"graha", "rsud"}
	for _, a := range histAccts {
		otherWinAccts = append(otherWinAccts, a.id)
	}
	lines := []string{"CCTV", "Fire alarm", "Videotron", "Signage", "Command center", "Videowall", "Maintenance"}
	referrers := []string{"amarta", "semen", "bsd", "panti", "baja", "hist-a04", "hist-a05"}

	order := []string{"ekspansi", "referral", "inbound", "tender", "cold"}
	n := 0
	ek, ow, ref := 0, 0, 0
	for _, src := range order {
		cnt, wins := h.Sources[src][0], h.Sources[src][1]
		for i := 0; i < cnt; i++ {
			d := deal{id: fmt.Sprintf("hist-d%02d", n+1), source: src, line: lines[n%len(lines)]}
			if i < wins {
				d.status = "won"
				if src == "ekspansi" {
					d.account = ekspansiAccts[ek%len(ekspansiAccts)]
					ek++
				} else {
					d.account = otherWinAccts[ow%len(otherWinAccts)]
					ow++
				}
			} else {
				d.status = "lost"
				d.account = histAccts[(n*7)%len(histAccts)].id
			}
			if src == "referral" {
				d.referrer = referrers[ref%len(referrers)]
				ref++
			}
			deals = append(deals, d)
			n++
		}
	}
	// Stakeholders & silence patterns: wins 15 multi / 5 single, losses 14 multi / 25 single;
	// silent after price revision: 4 wins, 10 losses.
	wi, li := 0, 0
	for i := range deals {
		if deals[i].status == "won" {
			deals[i].stakeholders = map[bool]int{true: 2 + wi%2, false: 1}[wi < 15]
			deals[i].silent = wi >= 16
			wi++
		} else {
			deals[i].stakeholders = map[bool]int{true: 2, false: 1}[li < 14]
			deals[i].silent = li >= 29
			li++
		}
	}
	// Timing. Pulse groups use the first 18 wins: 9 paid since ARC went live, 9 in the 6 months before.
	median := func(v []int) int { return v[len(v)/2] }
	pick := func(v []int, i int) int {
		if i < len(v) {
			return v[i]
		}
		return median(v)
	}
	var winIdx, lostIdx []int
	for i, d := range deals {
		if d.status == "won" {
			winIdx = append(winIdx, i)
		} else {
			lostIdx = append(lostIdx, i)
		}
	}
	govWins, entWins := []int{}, []int{}
	for _, i := range winIdx {
		if govOf[deals[i].account] {
			govWins = append(govWins, i)
		} else {
			entWins = append(entWins, i)
		}
	}
	cycles := map[int]int{}
	spread := func(idx []int, mean int) {
		offs := []int{-24, 24, -12, 12, -6, 6, -18, 18, -30, 30, 0}
		for k, i := range idx {
			o := offs[k%len(offs)]
			if k == len(idx)-1 && len(idx)%2 == 1 {
				o = 0
			}
			cycles[i] = mean + o
		}
	}
	spread(govWins, 96)
	spread(entWins, 54)
	for k, i := range winIdx {
		d := &deals[i]
		var paid time.Time
		var l2c, w2i, dso, w2b int
		switch {
		case k < 9:
			paid = arcStart.AddDate(0, 0, 4+k*9)
			l2c, w2i, dso = pick(h.L2CAfter, k), pick(h.W2IAfter, k), pick(h.DSOAfter, k)
			if k < len(h.W2BRecent) {
				w2b = h.W2BRecent[k]
			}
		case k < 18:
			j := k - 9
			paid = arcStart.AddDate(0, -6, 6+j*18)
			l2c, w2i, dso = pick(h.L2CBefore, j), pick(h.W2IBefore, j), pick(h.DSOBefore, j)
		default:
			paid = arcStart.AddDate(-1, 0, k*5)
			l2c, w2i, dso = 90, 18, 50
		}
		d.paid = t(paid)
		d.invoice = t(paid.AddDate(0, 0, -dso))
		d.won = t(d.invoice.AddDate(0, 0, -w2i))
		d.quotation = t(paid.AddDate(0, 0, -l2c))
		if w2b > 0 {
			d.bast = t(d.won.AddDate(0, 0, w2b))
		}
		d.lead = t(d.won.AddDate(0, 0, -cycles[i]))
		if d.lead.After(*d.quotation) {
			d.lead = t(d.quotation.AddDate(0, 0, -7))
		}
	}
	for k, i := range lostIdx {
		d := &deals[i]
		var lead time.Time
		switch {
		case k < len(h.RespAfter):
			lead = arcStart.AddDate(0, 0, 3+k*10)
			r := h.RespAfter[k]
			d.response = &r
		case k < len(h.RespAfter)+len(h.RespBefore):
			j := k - len(h.RespAfter)
			lead = arcStart.AddDate(0, -6, 5+j*20)
			r := h.RespBefore[j]
			d.response = &r
		default:
			lead = now.AddDate(0, -8-(k%14), 0)
		}
		d.lead = t(lead)
		d.lost = t(lead.AddDate(0, 0, 40+k%30))
	}
	// Referral deals inside the last 12 months: one per referrer; the rest older.
	ref = 0
	for i := range deals {
		d := &deals[i]
		if d.source != "referral" {
			continue
		}
		d.referrer = ""
		if d.lead != nil && !d.lead.Before(now.AddDate(-1, 0, 0)) {
			d.referrer = referrers[ref%len(referrers)]
			ref++
		}
	}
	for _, d := range deals {
		stage := s.stageID["Won"]
		if d.status == "lost" {
			stage = s.stageID["Lost"]
		}
		if err := s.exec(`INSERT INTO opportunities(id,account_id,name,expected_revenue,probability,stage_id,tags,source,status,lead_at,quotation_at,won_at,lost_at,invoice_at,bast_at,paid_at,
				first_response_minutes,historical,active_stakeholders,silent_after_revision,referrer_account_id,product_line,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,true,$18,$19,$20,$21,$22) ON CONFLICT (id) DO NOTHING`,
			d.id, d.account, d.line+" (historis)", 450000000, map[string]int{"won": 100, "lost": 0}[d.status], stage, []string{d.line}, d.source, d.status,
			d.lead, d.quotation, d.won, d.lost, d.invoice, d.bast, d.paid, d.response, d.stakeholders, d.silent, nilIfEmpty(d.referrer), d.line, now.AddDate(-2, 0, 0)); err != nil {
			return err
		}
	}
	// Legacy wins (> 24 months): second product line for expansion accounts, and older customers.
	legacy := []struct{ acc, line string }{}
	for i, a := range ekspansiAccts {
		legacy = append(legacy, struct{ acc, line string }{a, lines[(i+3)%len(lines)]})
	}
	for _, a := range histAccts[:3] {
		legacy = append(legacy, struct{ acc, line string }{a.id, "Maintenance"}, struct{ acc, line string }{a.id, "Fire alarm"})
	}
	for _, a := range legacyAccts {
		legacy = append(legacy, struct{ acc, line string }{a.id, "CCTV"})
	}
	for i, l := range legacy {
		won := now.AddDate(-2, -3-i%9, 0)
		if err := s.exec(`INSERT INTO opportunities(id,account_id,name,expected_revenue,probability,stage_id,tags,source,status,lead_at,won_at,historical,product_line,created_at)
			VALUES ($1,$2,$3,350000000,100,$4,$5,'inbound','won',$6,$7,true,$8,$7) ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("legacy-%02d", i+1), l.acc, l.line+" (legacy)", s.stageID["Won"], []string{l.line}, won.AddDate(0, -2, 0), won, l.line); err != nil {
			return err
		}
	}
	// The two Q3 wins carry pipeline attributes too.
	if err := s.exec(`UPDATE opportunities SET referrer_account_id='semen', active_stakeholders=3 WHERE id='won_cakra'`); err != nil {
		return err
	}
	if err := s.exec(`UPDATE opportunities SET active_stakeholders=2 WHERE id='won_salatiga'`); err != nil {
		return err
	}
	return s.llmHistory()
}

func nilIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// llmHistory records month-to-date LLM usage as daily aggregates per tier.
func (s *Seeder) llmHistory() error {
	monthStart := time.Date(s.C.Anchor.Year(), s.C.Anchor.Month(), 1, 23, 0, 0, 0, s.C.Anchor.Location())
	tiers := []struct {
		tier, provider, model, purpose string
		calls, in, out                 int
		cost                           float64
	}{
		{"light", "anthropic", "claude-haiku-4-5", "capture+hygiene (agregat harian)", 1900, 1520000, 285000, 2.945},
		{"heavy", "anthropic", "claude-sonnet-5", "deal/forecast/brief (agregat harian)", 60, 300000, 48000, 1.08},
		{"interactive", "anthropic", "claude-sonnet-5", "ask (agregat harian)", 10, 60000, 10000, 0.22},
	}
	for d := monthStart; d.Before(s.C.Anchor.Add(-12 * time.Hour)); d = d.AddDate(0, 0, 1) {
		for _, tr := range tiers {
			hash := fmt.Sprintf("seed-%s-%s", tr.tier, d.Format("2006-01-02"))
			if err := s.exec(`INSERT INTO llm_calls(tier,provider,model,tokens_in,tokens_out,cost_est,purpose,input_hash,duration_ms,ok,created_at)
				SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,true,$10 WHERE NOT EXISTS (SELECT 1 FROM llm_calls WHERE input_hash=$8)`,
				tr.tier, tr.provider, tr.model, tr.in, tr.out, tr.cost, tr.purpose, hash, tr.calls*900, d); err != nil {
				return err
			}
		}
	}
	return nil
}
