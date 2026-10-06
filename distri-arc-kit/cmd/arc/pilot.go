package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/pilot"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// runPilotCtl: arc ctl pilot start [--branch Semarang] | live | off | status | audit | snapshot [--week d] | export [--week d] [--out f]
func runPilotCtl(ctx context.Context, st *store.Store, c clock.Clock, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: arc ctl pilot start [--branch Semarang] | live | off | status | audit | snapshot [--week YYYY-MM-DD] | export [--week YYYY-MM-DD] [--out f]")
	}
	fs := flag.NewFlagSet("pilot "+args[0], flag.ExitOnError)
	branch := fs.String("branch", "", "pilot branch")
	week := fs.String("week", "", "any day of the week (YYYY-MM-DD)")
	out := fs.String("out", "", "CSV file (default stdout)")
	_ = fs.Parse(args[1:])
	svc := pilot.Service{St: st, Clock: c}
	weekRange := func() (time.Time, time.Time, error) {
		if *week == "" {
			return svc.Period(ctx)
		}
		t, err := time.ParseInLocation("2006-01-02", *week, clock.WIB)
		if err != nil {
			return t, t, err
		}
		f := pilot.Week(t)
		return f, f.AddDate(0, 0, 7), nil
	}
	switch args[0] {
	case "start", "live", "off":
		mode := map[string]string{"start": "shadow", "live": "live", "off": "off"}[args[0]]
		pp, err := svc.SetMode(ctx, mode, *branch, nil)
		if err != nil {
			return err
		}
		fmt.Printf("pilot %s · cabang %s · mulai %s · bayangan %d hari\n", pp.Mode, pp.Branch, pp.StartedAt, pp.ShadowDays)
		return nil
	case "status", "audit":
		from, to, err := weekRange()
		if err != nil {
			return err
		}
		r, err := svc.Build(ctx, from, to)
		if err != nil {
			return err
		}
		if args[0] == "status" {
			fmt.Printf("pilot %s · cabang %s · hari ke-%d · %s s.d. %s\n", r.Mode, r.Branch, r.Day, clock.DayMonth(r.From), clock.DayMonth(r.To.AddDate(0, 0, -1)))
			fmt.Printf("%-14s %6s %9s %7s %7s %9s %10s %s\n", "agen", "saran", "disetujui", "diedit", "ditolak", "diterima", "median", "otonomi")
			for _, a := range r.Agents {
				fmt.Printf("%-14s %6d %9d %7d %7d %9s %10s %s\n", a.Agent, a.Proposed, a.Approved, a.Edited, a.Rejected, pctStr(a.AcceptPct), minStr(a.MedianMin), unlockStr(a))
			}
			fmt.Printf("order tepat jadwal %d%% (target %d%%) · DSO %d hari (target %d) · lewat jadwal %d, tertangkap sebelum churn %d, churn %d\n",
				r.OnSchedule, r.Targets.OnSchedulePct, r.DSO, r.Targets.DSODays, r.AtRisk, r.Caught, r.Churned)
		}
		bad := 0
		for _, ch := range r.Audit {
			mark := "  OK  "
			if ch.Violations > 0 {
				mark, bad = " FAIL ", bad+1
			}
			fmt.Printf("[%s] %-62s %d\n", mark, ch.Label, ch.Violations)
		}
		if bad > 0 {
			return fmt.Errorf("audit pilot: %d pemeriksaan gagal", bad)
		}
		return nil
	case "snapshot":
		t := c.Now()
		if *week != "" {
			var err error
			if t, err = time.ParseInLocation("2006-01-02", *week, clock.WIB); err != nil {
				return err
			}
		}
		r, err := svc.Snapshot(ctx, t)
		if err != nil {
			return err
		}
		fmt.Printf("snapshot minggu %s · cabang %s · %d keputusan · audit %v\n", r.From.Format("2006-01-02"), r.Branch, r.Decisions, r.AuditOK)
		return nil
	case "export":
		from, to, err := weekRange()
		if err != nil {
			return err
		}
		r, err := svc.Build(ctx, from, to)
		if err != nil {
			return err
		}
		if *out == "" {
			_, err = os.Stdout.Write(pilot.CSV(r))
			return err
		}
		return os.WriteFile(*out, pilot.CSV(r), 0o600)
	}
	return fmt.Errorf("pilot %s: unknown", args[0])
}

func pctStr(p *int) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%d%%", *p)
}

func minStr(p *int) string {
	if p == nil {
		return "—"
	}
	if *p >= 120 {
		return fmt.Sprintf("%.1f jam", float64(*p)/60)
	}
	return fmt.Sprintf("%d mnt", *p)
}

func unlockStr(a pilot.AgentStat) string {
	switch {
	case a.Unlocked:
		return "dibuka"
	case a.Eligible:
		return "boleh dibuka · " + a.EligibleNote
	}
	return "approve · " + a.EligibleNote
}

// importInternal reads internal_numbers from CSV: wa_number,label,department,is_sales (header optional).
func importInternal(ctx context.Context, st *store.Store, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.FieldsPerRecord = -1
	n := 0
	for {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return n, err
		}
		num := wa.Digits(rec[0])
		if num == "" || strings.EqualFold(rec[0], "wa_number") {
			continue
		}
		get := func(i int) *string {
			if i < len(rec) && strings.TrimSpace(rec[i]) != "" {
				v := strings.TrimSpace(rec[i])
				return &v
			}
			return nil
		}
		isSales := len(rec) > 3 && strings.EqualFold(strings.TrimSpace(rec[3]), "true")
		if err := st.Q.UpsertInternalNumber(ctx, gen.UpsertInternalNumberParams{WaNumber: num, Label: get(1), Department: get(2), IsSales: isSales}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
