package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/odoo"
	"distri-arc/internal/outbox"
	"distri-arc/internal/pilot"
	"distri-arc/internal/proposals"
	"distri-arc/internal/store"
	"distri-arc/internal/wa"
)

// simClock is the rehearsal's day-by-day clock.
type simClock struct{ t time.Time }

func (c *simClock) Now() time.Time { return c.t }

// rejectRate is how often the rehearsal's "humans" reject an agent's proposals (deterministic per proposal), chosen
// so some agents qualify for autonomy after two weeks and others do not.
var rejectRate = map[string]uint32{"AI Order": 5, "AI Follow-up": 10, "AI Penagihan": 12, "AI Kredit": 30, "AI Stok": 35, "AI Prospek": 20}

func roll(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32() % 100
}

// runRehearsal (dev only): a pilot rehearsal on the current database — N days from ARC_NOW, 14 in shadow mode, the
// rest live. Each day: metrics + snapshot, one Orchestrator cycle, deterministic human decisions (some rejected, ~15%
// left open), orders that follow approved follow-ups two days later, outbox delivered by fakes, the pilot audit, and the
// weekly pilot snapshot on Mondays. It proves the pilot tooling end to end; its numbers are not pilot results.
func runRehearsal(ctx context.Context, cfg config.Config, st *store.Store, start clock.Clock, log *slog.Logger, args []string) error {
	if !cfg.IsDev() {
		return errors.New("rehearse writes simulated decisions and orders: APP_ENV=dev only (use a scratch database)")
	}
	fs := flag.NewFlagSet("pilot rehearse", flag.ExitOnError)
	days := fs.Int("days", 21, "days to simulate")
	_ = fs.Parse(args)
	c := &simClock{t: clock.Today(start.Now()).Add(7 * time.Hour)}
	orch := newOrchestrator(ctx, cfg, st, c, log)
	orch.OdooWrite = true
	svc := pilot.Service{St: st, Clock: c}
	ms := dealersvc.New(st, c)
	fo, err := odoo.NewFake(db.Seed, "seed/odoo")
	if err != nil {
		return err
	}
	fo.Write = true
	tr := wa.NewFake()
	var sam proposals.Decider
	if err := st.Pool.QueryRow(ctx, "select id, name from sales_users where role = 'ceo'").Scan(&sam.SalesUserID, &sam.Name); err != nil {
		return err
	}
	sam.Role, sam.Email = "ceo", "sam@gsi.co.id"
	if _, err := svc.SetMode(ctx, "shadow", "Semarang", nil); err != nil {
		return err
	}
	pp, _ := svc.SetMode(ctx, "shadow", "", nil)
	shadowDays := pp.ShadowDays
	type pend struct {
		dealer uuid.UUID
		at     time.Time
	}
	var orders []pend
	fmt.Printf("gladi pilot %d hari dari %s · bayangan %d hari · cabang %s\n", *days, clock.DayMonth(c.t), shadowDays, pp.Branch)
	fmt.Printf("%-8s %-7s %6s %7s %7s %6s %7s %6s %s\n", "hari", "mode", "saran", "setuju", "tolak", "buka", "kirim", "order", "audit")
	for d := range *days {
		day := clock.Today(start.Now()).AddDate(0, 0, d)
		c.t = day.Add(7 * time.Hour)
		mode := "shadow"
		if d == shadowDays {
			if _, err := svc.SetMode(ctx, "live", "", nil); err != nil {
				return err
			}
			for _, a := range domain.AgentNames {
				if _, err := svc.Unlock(ctx, a, nil); err == nil {
					fmt.Printf("         → otonomi dibuka: %s\n", a)
				}
			}
		}
		if d >= shadowDays {
			mode = "live"
		}
		// orders that follow yesterday's approved follow-ups (as the Odoo sync would bring them)
		placed := 0
		var keep []pend
		for _, o := range orders {
			if o.at.After(c.t) {
				keep = append(keep, o)
				continue
			}
			if _, err := st.Pool.Exec(ctx, `insert into orders (dealer_id, number, state, ordered_at, confirmed_at, total, margin_pct, lines, source_system, source_id, source_write_date)
				select dealer_id, 'SIM-' || left(gen_random_uuid()::text, 8), 'kirim', $2, $2, total, margin_pct, lines, 'sim', 'sim:' || gen_random_uuid(), $2
				from orders where dealer_id = $1 and state <> 'cancel' order by confirmed_at desc nulls last limit 1`, o.dealer, o.at); err != nil {
				return err
			}
			placed++
		}
		orders = keep
		if _, err := ms.Snapshot(ctx); err != nil {
			return err
		}
		if _, err := orch.Run(ctx, domain.Scope{Kind: "all"}, domain.Trigger{Source: "schedule", By: "gladi", Via: "api"}); err != nil {
			return fmt.Errorf("day %d cycle: %w", d, err)
		}
		rows, err := st.Pool.Query(ctx, `select id, agent, kind, title, dealer_id from proposals where status = 'proposed' and created_at >= $1 order by created_at, id`, day)
		if err != nil {
			return err
		}
		type open struct {
			id          uuid.UUID
			agent, kind string
			title       string
			dealer      *uuid.UUID
		}
		var list []open
		for rows.Next() {
			var o open
			if err := rows.Scan(&o.id, &o.agent, &o.kind, &o.title, &o.dealer); err != nil {
				rows.Close()
				return err
			}
			list = append(list, o)
		}
		rows.Close()
		approved, rejected := 0, 0
		for i, o := range list {
			r := roll(o.id.String())
			if r >= 85 {
				continue // left for later: expires
			}
			c.t = day.Add(7*time.Hour + time.Duration(15+i*7)*time.Minute)
			agent := strings.Split(o.agent, " + ")[0]
			dec := proposals.Decision{Decision: "approve"}
			if roll(o.title+o.kind) < rejectRate[agent] {
				dec = proposals.Decision{Decision: "reject", Reason: "konteks_kurang"}
			}
			out, err := proposals.Decide(ctx, st, nil, c, true, o.id, sam, dec)
			if err != nil {
				continue // e.g. a guard refusing the option; counted as undecided
			}
			if dec.Decision == "reject" {
				rejected++
				continue
			}
			approved++
			if o.dealer != nil && (o.kind == domain.KindFollowup || o.kind == domain.KindSODraft) && roll(o.id.String()+"order") < 60 {
				orders = append(orders, pend{dealer: *o.dealer, at: day.AddDate(0, 0, 2).Add(10 * time.Hour)})
			}
			_ = out
		}
		// deliver whatever the outbox holds (fakes); shadow rows are never sent
		c.t = day.Add(17 * time.Hour)
		sender := outbox.NewSender(st, tr, c, outbox.Rules{DailyCapOverride: 40}).WithOdoo(fo)
		ids, err := st.Q.PendingOutboxIDs(ctx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			_, _ = sender.Send(ctx, id)
		}
		var sent, auto int
		_ = st.Pool.QueryRow(ctx, "select count(*) from outbox where status = 'sent' and sent_at >= $1 and sent_at < $2", day, day.AddDate(0, 0, 1)).Scan(&sent)
		_ = st.Pool.QueryRow(ctx, "select count(*) from proposals where autonomy = 'auto' and created_at >= $1 and created_at < $2", day, day.AddDate(0, 0, 1)).Scan(&auto)
		rep, err := svc.Build(ctx, day, day.AddDate(0, 0, 1))
		if err != nil {
			return err
		}
		audit := "bersih"
		if !rep.AuditOK {
			audit = "GAGAL"
			for _, ch := range rep.Audit {
				if ch.Violations > 0 {
					audit += fmt.Sprintf(" %s=%d", ch.Key, ch.Violations)
				}
			}
		}
		fmt.Printf("%-8s %-7s %6d %7d %7d %6d %7d %6d %s\n", clock.DayMonth(day), mode, len(list), approved, rejected, auto, sent, placed, audit)
		if day.Weekday() == time.Sunday {
			if _, err := svc.Snapshot(ctx, day); err != nil {
				return err
			}
		}
	}
	from := clock.Today(start.Now())
	rep, err := svc.Build(ctx, from, clock.Today(c.t).AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	fmt.Printf("\nringkasan %s – %s · %d keputusan · %s diterima · tepat jadwal %d%% · DSO %d hr · lewat jadwal %d, tertangkap %d, churn %d · audit %v\n",
		clock.DayMonth(rep.From), clock.DayMonth(rep.To.AddDate(0, 0, -1)), rep.Decisions, pctStr(rep.AcceptPct), rep.OnSchedule, rep.DSO, rep.AtRisk, rep.Caught, rep.Churned, rep.AuditOK)
	for _, a := range rep.Agents {
		fmt.Printf("  %-13s saran %3d · diterima %5s · %s\n", a.Agent, a.Proposed, pctStr(a.AcceptPct), unlockStr(a))
	}
	if !rep.AuditOK {
		return errors.New("gladi: audit pilot gagal")
	}
	return nil
}
