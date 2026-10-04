package seed

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

func (s *Seeder) cash() error {
	idByAid := map[string]string{}
	for aid, m := range s.F.Extra.L2CMeta {
		idByAid[aid] = m.ID
	}
	for _, r := range s.F.L2C {
		aid := r.Aid
		if aid == "" {
			aid = "_amarta"
		}
		m := s.F.Extra.L2CMeta[aid]
		stage := domain.L2CStages[r.Stage-1]
		entered := s.C.Anchor.AddDate(0, 0, -r.Days)
		var inv any
		if m.Invoice != "" {
			inv = m.Invoice
		}
		var paid any
		if stage == "lunas" {
			paid = s.C.Anchor
		}
		if err := s.exec(`INSERT INTO cash_items(id,account_id,account_name,project,so_id,project_id,value,stage,stage_entered_at,benchmark_days,owner_user_id,note,invoice_id,paid_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT (id) DO NOTHING`,
			m.ID, m.Account, r.Acc, r.Proj, r.SO, m.ProjectID, r.Value, stage, entered, r.Bench, s.user(r.Owner), r.Note, inv, paid); err != nil {
			return err
		}
	}
	for _, in := range s.F.Extra.Invoices {
		cat := in.Category
		if cat == "" {
			cat = "project"
		}
		residual := in.Amount
		var paid any
		if in.Paid != "" {
			residual = 0
			paid = s.C.At(in.Paid)
		}
		var spm any
		if in.SPM != "" {
			spm = s.C.At(in.SPM)
		}
		name := in.Account
		_ = s.tx.QueryRow(s.ctx, `SELECT name FROM accounts WHERE id=$1`, in.Account).Scan(&name)
		if err := s.exec(`INSERT INTO invoices(id,number,so_id,account_id,account_name,category,label,amount,residual,invoice_date,due_date,paid_at,pay_pattern,spm_submitted_at,is_government)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT (id) DO NOTHING`,
			in.ID, in.Number, in.SO, in.Account, name, cat, in.Label, in.Amount, residual, s.C.At(in.InvoiceDate), s.C.At(in.Due), paid, in.Pattern, spm, in.Gov); err != nil {
			return err
		}
	}
	for _, x := range s.F.Extra.CashForecastItems {
		if x.Kind != "expected" {
			continue
		}
		var ci any
		if x.CashItem != "" {
			ci = x.CashItem
		}
		if err := s.exec(`INSERT INTO expected_receipts(id,account_id,label,detail,amount,trigger,cash_item_id) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
			x.ID, x.Account, x.Label, x.Detail, x.Amount, x.Trigger, ci); err != nil {
			return err
		}
	}
	for _, ph := range s.F.Extra.PaymentHistory {
		for i, ref := range ph.Refs {
			name, _ := ref[0].(string)
			days := int(ref[1].(float64))
			paid := s.C.Anchor.AddDate(0, -3*(len(ph.Refs)-i), 0)
			if err := s.exec(`INSERT INTO payment_history(account_id,invoice_ref,amount,term_days,days_to_pay,paid_on) VALUES ($1,$2,$3,30,$4,$5) ON CONFLICT DO NOTHING`,
				ph.Account, name, 100000000, days, paid); err != nil {
				return err
			}
		}
	}
	for _, c := range s.F.Extra.CreditProfiles {
		if err := s.exec(`INSERT INTO credit_profiles(account_id,credit_limit,open_receivable,avg_days_to_pay,overdue_note,registered_phone) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			c.Account, c.Limit, c.Open, c.AvgDays, c.OverdueNote, c.Phone); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) installed() error {
	all := map[string]Installed{}
	for k, v := range s.F.Installed {
		all[k] = v
	}
	for k, v := range s.F.Extra.InstalledExtra {
		all[k] = v
	}
	for acc, in := range all {
		for _, x := range in.Inst {
			end := x.WarrantyEnd
			if end == "" {
				end = s.F.Extra.WarrantyEnds[acc]
			}
			var we any
			if end != "" {
				we = s.C.At(end)
			}
			units := 0
			fmt.Sscanf(digitsAfterSpace(x.S), "%d", &units)
			if err := s.exec(`INSERT INTO installed_systems(account_id,system,installed_year,warranty_end,warranty_label,service_contract,units) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
				acc, x.S, x.Y, we, x.W, x.C, units); err != nil {
				return err
			}
		}
		for i, w := range in.WS {
			if err := s.exec(`INSERT INTO whitespace(account_id,product_line,status,value,why,seq) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
				acc, w.P, w.St, w.V, w.Why, i); err != nil {
				return err
			}
		}
	}
	return nil
}

func digitsAfterSpace(s string) string {
	for _, f := range strings.Fields(s) {
		if f[0] >= '0' && f[0] <= '9' {
			return f
		}
	}
	return "0"
}

var funnelStages = []string{"masuk", "teridentifikasi", "relevan", "pain_point", "lead", "penawaran", "won"}

func (s *Seeder) inbound() error {
	reached := map[string]int{"in1": 3, "in3": 1, "in2": 2, "in4": 2}
	for _, x := range s.F.Inbound {
		status := map[string]string{"identified": "identified", "unknown": "unknown", "notprospect": "not_prospect"}[x.Status]
		phone := s.F.Extra.InboundFull[x.ID]
		ident := map[string]any{"name": x.Ident.Name, "role": x.Ident.Role, "company": x.Ident.Company, "sources": x.Ident.Sources}
		conf := 0.0
		if x.Score != nil {
			conf = float64(*x.Score) / 100
		}
		ident["confidence"] = conf
		received := s.C.At(s.F.Extra.InboundReceived[x.ID])
		if err := s.exec(`INSERT INTO inbound_contacts(id,phone,phone_norm,first_message,via_user_id,received_at,identification,overview,fit_score,status,solutions,pain_questions,expires_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (id) DO NOTHING`,
			x.ID, phone, domain.NormalizePhone(phone), x.First, s.user(x.Via), received, storage.JSONObj(ident), x.Overview, x.Score, status,
			storage.JSON(x.Solutions), storage.JSON(x.Questions), received.AddDate(0, 0, 90)); err != nil {
			return err
		}
		for i := 0; i < reached[x.ID]; i++ {
			if err := s.exec(`INSERT INTO funnel_events(inbound_id,subject_key,stage,channel,at) VALUES ($1,$1,$2,'wa',$3) ON CONFLICT DO NOTHING`,
				x.ID, funnelStages[i], received.Add(time.Duration(i*6)*time.Minute)); err != nil {
				return err
			}
		}
	}
	// Synthetic inbound subjects for the rest of the month (aggregated history of the funnel).
	depths := []struct{ depth, n int }{{7, 1}, {6, 3}, {5, 3}, {4, 2}, {3, 4}, {2, 15}, {1, 14}}
	monthStart := time.Date(s.C.Anchor.Year(), s.C.Anchor.Month(), 1, 9, 0, 0, 0, domain.Jakarta)
	days := s.C.Anchor.Day()
	idx := 0
	for _, d := range depths {
		for k := 0; k < d.n; k++ {
			key := fmt.Sprintf("fx-%02d", idx)
			at := monthStart.AddDate(0, 0, idx%days)
			ch := s.F.Extra.Funnel.Channels[idx%len(s.F.Extra.Funnel.Channels)]
			for st := 0; st < d.depth; st++ {
				var off time.Duration
				switch {
				case st == 1:
					off = 6 * time.Minute
				case st >= 2 && st < 4:
					off = time.Duration(st) * time.Hour
				case st == 4:
					off = 31 * time.Hour
				case st > 4:
					off = time.Duration(st*3) * 24 * time.Hour
				}
				if err := s.exec(`INSERT INTO funnel_events(subject_key,stage,channel,at) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
					key, funnelStages[st], ch, at.Add(off)); err != nil {
					return err
				}
			}
			idx++
		}
	}
	return nil
}

func (s *Seeder) internal() error {
	for _, in := range s.F.Internal {
		phone := s.F.Extra.InternalFull[in.N]
		src := strings.ToLower(in.Src)
		if err := s.exec(`INSERT INTO internal_numbers(name,phone,phone_norm,unit,branch,source,confirmed_by) VALUES ($1,$2,$3,$4,$5,$6,'seed') ON CONFLICT (phone_norm) DO NOTHING`,
			in.N, phone, domain.NormalizePhone(phone), in.Unit, in.Branch, src); err != nil {
			return err
		}
	}
	for _, sp := range s.F.Suspects {
		phone := s.F.Extra.SuspectsFull[sp.No]
		groups := 2
		if strings.Contains(sp.Why, "3 grup") {
			groups = 3
		}
		if err := s.exec(`INSERT INTO internal_suspects(phone,phone_norm,name_hint,reason,group_count) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (phone_norm) DO NOTHING`,
			phone, domain.NormalizePhone(phone), strings.Trim(sp.N, "“”"), sp.Why, groups); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) setting(key string, v any) error {
	return s.exec(`INSERT INTO settings(key,value) VALUES ($1,$2) ON CONFLICT (key) DO NOTHING`, key, storage.JSONObj(v))
}

func (s *Seeder) ops() error {
	for i, r := range s.F.Extra.PrivacyRules {
		if err := s.exec(`INSERT INTO privacy_rules(id,title,detail,enabled,locked,seq) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`,
			r.ID, r.Title, r.Detail, r.Enabled, r.Locked, i); err != nil {
			return err
		}
	}
	for i, c := range s.F.Extra.Connectors {
		text := c.Text
		if text == "" {
			text = "#fff"
		}
		if err := s.exec(`INSERT INTO connectors(id,name,subtitle,logo,color,text_color,grp,seq) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`,
			c.ID, c.Name, c.Sub, c.Logo, c.Color, text, c.Grp, i); err != nil {
			return err
		}
	}
	for _, c := range s.F.Extra.AIClients {
		var last any
		if c.Last != "" {
			last = s.C.At(c.Last)
		}
		if err := s.exec(`INSERT INTO ai_clients(id,name,logo,color,description,users_count,last_seen_at) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
			c.ID, c.Name, c.Logo, c.Color, c.Desc, c.Users, last); err != nil {
			return err
		}
	}
	for i, k := range s.F.Extra.APIKeys {
		secret := make([]byte, 18)
		_, _ = rand.Read(secret)
		full := k.Prefix + hex.EncodeToString(secret)
		sum := sha256.Sum256([]byte(full))
		var last any
		if k.Last != "" {
			last = s.C.At(k.Last)
		}
		if err := s.exec(`INSERT INTO api_keys(id,name,prefix,key_hash,scopes,owner_user_id,rate_limit_rpm,calls_count,last_used_at,created_at) VALUES ($1,$2,$3,$4,$5,'sam',$6,$7,$8,$9) ON CONFLICT (id) DO NOTHING`,
			k.ID, k.Name, k.Prefix, hex.EncodeToString(sum[:]), k.Scopes, k.RPM, k.Calls, last, s.C.Anchor.AddDate(0, -1, 0).Add(-time.Duration(i)*time.Minute)); err != nil {
			return err
		}
	}
	for i, g := range [][3]string{{"accounts", "Akun & relasi", ""}, {"deals", "Deal & pipeline", "sinkron Odoo"}, {"chat", "Chat & WhatsApp", ""}, {"cash", "Cash", ""}, {"actions", "Tindakan & kebijakan", ""}} {
		if err := s.exec(`INSERT INTO mcp_tool_groups(id,label,note,seq) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`, g[0], g[1], g[2], i); err != nil {
			return err
		}
	}
	for _, e := range s.F.Extra.Calendar {
		var acc any
		if e.Account != "" {
			acc = e.Account
		}
		if err := s.exec(`INSERT INTO calendar_events(id,raw_ref,title,starts_at,duration_min,location,attendees,account_id,internal,prep_status,prep_pills,owner_user_id,source)
			VALUES ($1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'manual') ON CONFLICT (id) DO NOTHING`,
			e.ID, e.Title, s.C.At(e.Start), e.Duration, e.Location, e.Attendees, acc, e.Internal, e.Prep, storage.JSON(e.Pills), e.Owner); err != nil {
			return err
		}
	}
	for _, t := range s.F.Extra.Tenders {
		if err := s.exec(`INSERT INTO tenders(id,title,agency,source,hps,deadline,description,reasons) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`,
			t.ID, t.Title, t.Agency, t.Source, t.HPS, s.C.At(t.Deadline), t.Description, t.Reasons); err != nil {
			return err
		}
	}
	for user, st := range s.F.Extra.TeamStats {
		if st.ResponseMin > 0 {
			if err := s.exec(`INSERT INTO metric_snapshots(key,subject,value) VALUES ('response_min',$1,$2),('followup_on_time',$1,$3) ON CONFLICT DO NOTHING`, user, st.ResponseMin, st.FollowupOnTime); err != nil {
				return err
			}
		}
	}
	settings := map[string]any{
		"wa_mode":              "cloud",
		"wa_history_days":      30,
		"wa_unlisted_groups":   s.F.Extra.UnlistedGroups,
		"commit_accuracy":      s.F.Extra.CommitAccuracy,
		"agent_feed":           s.F.Extra.AgentFeed,
		"today_activity":       s.F.Extra.TodayActivity,
		"arc_active_since":     s.C.At("2026-07-01").Format(time.RFC3339),
		"funnel_baseline":      map[string]any{"identify_minutes": 2880, "ident_to_lead_days": 5, "avg_lead_value": 290000000},
		"routing":              map[string]string{"light": "Claude Haiku 4.5", "heavy": "Claude Sonnet 5", "interactive": "Claude Sonnet 5", "fallback": "OpenAI"},
		"wa_history_estimates": s.F.WAHistory,
		"screens":              s.F.Screens,
		"tender_keywords":      s.F.Extra.TenderKeywords,
		"llm_cost_month_idr":   1900000,
	}
	for k, v := range settings {
		if err := s.setting(k, v); err != nil {
			return err
		}
	}
	return nil
}
